-- Slice 1.3 needs the source's expected refresh interval to reconstruct freshness
-- deterministically after restart. No observation ingestion existed before this
-- migration, but the bounded backfill keeps manually inserted development rows
-- migratable.
ALTER TABLE observations
    ADD COLUMN expected_refresh_seconds integer;

UPDATE observations
SET expected_refresh_seconds = LEAST(
    86400,
    GREATEST(1, EXTRACT(EPOCH FROM (expires_at - observed_at))::integer)
);

ALTER TABLE observations
    ALTER COLUMN expected_refresh_seconds SET NOT NULL,
    ADD CONSTRAINT observations_expected_refresh_bounds
        CHECK (expected_refresh_seconds BETWEEN 1 AND 86400),
    ADD CONSTRAINT observations_expiry_order
        CHECK (expires_at >= observed_at);

-- A source may omit its UUID, so the normalized delivery key supplies retry
-- idempotency. A conflicting payload for the same key is rejected by the service.
CREATE UNIQUE INDEX observations_delivery_key_idx
    ON observations (integration_id, resource_id, check_type, observed_at);

ALTER TABLE current_health
    ADD COLUMN unknown_at timestamptz,
    ADD CONSTRAINT current_health_transition_order
        CHECK (unknown_at IS NULL OR stale_at IS NULL OR unknown_at >= stale_at);

CREATE INDEX current_health_unknown_at_idx ON current_health (unknown_at);
