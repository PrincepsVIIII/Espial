ALTER TABLE adapter_instances
    ADD COLUMN protocol_version text,
    ADD COLUMN last_stopped_at timestamptz,
    ADD COLUMN last_error_at timestamptz,
    ADD COLUMN consecutive_failures integer NOT NULL DEFAULT 0,
    ADD COLUMN next_restart_at timestamptz,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
    ADD CONSTRAINT adapter_instances_protocol_version_format
        CHECK (protocol_version IS NULL OR protocol_version ~ '^[0-9]+\.[0-9]+$'),
    ADD CONSTRAINT adapter_instances_failure_count_nonnegative
        CHECK (consecutive_failures >= 0),
    ADD CONSTRAINT adapter_instances_error_code_format
        CHECK (last_error_code IS NULL OR last_error_code ~ '^[a-z][a-z0-9_.-]{0,126}$');

CREATE INDEX adapter_instances_next_restart_idx
    ON adapter_instances (next_restart_at)
    WHERE state = 'unhealthy' AND next_restart_at IS NOT NULL;
