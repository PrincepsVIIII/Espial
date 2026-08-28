-- Current health is a per-resource, per-check projection. A resource can have
-- independent availability, capacity, certificate, or future check streams, and
-- a newer stream must never replace another stream's current observation.
ALTER TABLE current_health DROP CONSTRAINT current_health_pkey;
ALTER TABLE current_health ADD COLUMN check_type text;

DELETE FROM current_health;

WITH ranked AS (
    SELECT o.*,
        row_number() OVER (
            PARTITION BY o.resource_id, o.check_type
            ORDER BY o.observed_at DESC, o.received_at DESC, o.id DESC
        ) AS rank
    FROM observations o
), latest AS (
    SELECT * FROM ranked WHERE rank = 1
), positive AS (
    SELECT resource_id, check_type, max(observed_at) FILTER (
        WHERE observed_state IN ('healthy', 'warning', 'critical', 'maintenance', 'disabled')
    ) AS last_success_at
    FROM observations
    GROUP BY resource_id, check_type
), projected AS (
    SELECT latest.*, positive.last_success_at,
        latest.observed_at + make_interval(secs => GREATEST(latest.expected_refresh_seconds / 2, 30)) + make_interval(secs => latest.expected_refresh_seconds) AS stale_at,
        latest.observed_at + make_interval(secs => latest.expected_refresh_seconds * 3) AS unknown_at
    FROM latest
    LEFT JOIN positive USING (resource_id, check_type)
)
INSERT INTO current_health (
    resource_id, check_type, state, reason, observation_id, observed_at,
    last_success_at, stale_at, unknown_at, updated_at
)
SELECT resource_id, check_type,
    CASE
        WHEN observed_state IN ('disabled', 'unknown') THEN observed_state
        WHEN now() >= unknown_at THEN 'unknown'
        WHEN now() >= stale_at THEN 'stale'
        ELSE observed_state
    END,
    CASE
        WHEN observed_state = 'disabled' THEN summary
        WHEN observed_state = 'unknown' THEN summary
        WHEN now() >= unknown_at THEN 'observation exceeded the unknown freshness threshold'
        WHEN now() >= stale_at THEN 'observation exceeded the expected refresh window'
        ELSE summary
    END,
    id, observed_at, last_success_at,
    CASE WHEN observed_state IN ('disabled', 'unknown') THEN NULL ELSE stale_at END,
    CASE WHEN observed_state IN ('disabled', 'unknown') THEN NULL ELSE unknown_at END,
    now()
FROM projected;

ALTER TABLE current_health
    ALTER COLUMN check_type SET NOT NULL,
    ADD CONSTRAINT current_health_check_type_format
        CHECK (check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'),
    ADD PRIMARY KEY (resource_id, check_type);

-- Resource-level reads deliberately select one deterministic, most-severe check
-- while exact-check consumers read current_health directly.
CREATE VIEW current_resource_health AS
SELECT DISTINCT ON (resource_id) *
FROM current_health
ORDER BY resource_id,
    CASE state
        WHEN 'critical' THEN 6
        WHEN 'warning' THEN 5
        WHEN 'unknown' THEN 4
        WHEN 'stale' THEN 3
        WHEN 'maintenance' THEN 2
        WHEN 'healthy' THEN 1
        WHEN 'disabled' THEN 0
        ELSE -1
    END DESC,
    observed_at DESC NULLS LAST,
    updated_at DESC,
    check_type ASC;
