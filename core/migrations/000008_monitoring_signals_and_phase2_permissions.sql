-- Phase 2 consumes durable monitoring signals instead of the in-memory SSE hub.
-- Permission updates union new names into built-in roles and leave custom roles
-- untouched.
UPDATE roles
SET permissions = (
    SELECT jsonb_agg(permission ORDER BY permission)
    FROM (
        SELECT DISTINCT jsonb_array_elements_text(
            permissions || '["incidents:read", "webpages:read"]'::jsonb
        ) AS permission
    ) merged
)
WHERE name IN ('viewer', 'operator');

UPDATE roles
SET permissions = (
    SELECT jsonb_agg(permission ORDER BY permission)
    FROM (
        SELECT DISTINCT jsonb_array_elements_text(
            permissions || '[
                "incidents:read",
                "webpages:read",
                "incident_rules:manage",
                "suppressions:manage",
                "notification_destinations:manage",
                "website_monitors:manage"
            ]'::jsonb
        ) AS permission
    ) merged
)
WHERE name = 'administrator';

CREATE TABLE monitoring_signals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_key text NOT NULL UNIQUE CHECK (length(source_key) BETWEEN 1 AND 256),
    kind text NOT NULL CHECK (kind IN ('observation', 'freshness')),
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    observation_id uuid REFERENCES observations(id) ON DELETE SET NULL,
    check_type text NOT NULL CHECK (check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'),
    state text NOT NULL CHECK (
        state IN ('healthy', 'warning', 'critical', 'unknown', 'stale', 'maintenance', 'disabled')
    ),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 1024),
    reason_code text CHECK (
        reason_code IS NULL OR reason_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    available_at timestamptz NOT NULL DEFAULT now(),
    claimed_until timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    processed_at timestamptz,
    dead_lettered_at timestamptz,
    last_error_code text CHECK (
        last_error_code IS NULL OR last_error_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    CHECK (processed_at IS NULL OR dead_lettered_at IS NULL)
);

CREATE INDEX monitoring_signals_due_idx
    ON monitoring_signals (available_at, occurred_at, id)
    WHERE processed_at IS NULL AND dead_lettered_at IS NULL;

CREATE INDEX monitoring_signals_resource_time_idx
    ON monitoring_signals (resource_id, check_type, occurred_at DESC, id DESC);

