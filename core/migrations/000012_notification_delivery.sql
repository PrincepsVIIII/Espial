CREATE TABLE notification_destinations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 128),
    destination_type text NOT NULL DEFAULT 'mattermost'
        CHECK (destination_type = 'mattermost'),
    enabled boolean NOT NULL DEFAULT true,
    endpoint_host text NOT NULL CHECK (
        length(endpoint_host) BETWEEN 1 AND 253
        AND endpoint_host = lower(endpoint_host)
        AND endpoint_host !~ '[/?#@]'
    ),
    endpoint_port integer NOT NULL DEFAULT 443 CHECK (endpoint_port BETWEEN 1 AND 65535),
    endpoint_path_prefix text NOT NULL CHECK (
        length(endpoint_path_prefix) BETWEEN 1 AND 256
        AND left(endpoint_path_prefix, 1) = '/'
        AND endpoint_path_prefix ~ '^/[A-Za-z0-9._~/-]{0,255}$'
        AND endpoint_path_prefix !~ '\.\.'
    ),
    secret_reference text NOT NULL CHECK (
        length(secret_reference) BETWEEN 1 AND 128
        AND secret_reference ~ '^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$'
    ),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notification_destinations_enabled_idx
    ON notification_destinations (destination_type, id) WHERE enabled;

CREATE TABLE notification_intents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_event_id uuid REFERENCES incident_timeline(id) ON DELETE CASCADE,
    incident_id uuid REFERENCES incidents(id) ON DELETE CASCADE,
    destination_id uuid NOT NULL REFERENCES notification_destinations(id) ON DELETE RESTRICT,
    event_kind text NOT NULL CHECK (
        event_kind IN ('detected', 'severity_changed', 'recurrence', 'recovered', 'test')
    ),
    is_test boolean NOT NULL DEFAULT false,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 256),
    summary text NOT NULL CHECK (length(summary) BETWEEN 1 AND 2048),
    severity text CHECK (severity IS NULL OR severity IN ('warning', 'critical')),
    incident_status text CHECK (
        incident_status IS NULL OR incident_status IN (
            'open', 'acknowledged', 'investigating', 'recovered', 'resolved'
        )
    ),
    event_occurred_at timestamptz NOT NULL,
    state text NOT NULL CHECK (
        state IN (
            'queued', 'attempting', 'delivered', 'retry_wait',
            'failed', 'dead_letter', 'suppressed'
        )
    ),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 6),
    available_at timestamptz NOT NULL DEFAULT now(),
    claimed_until timestamptz,
    claim_token uuid,
    delivered_at timestamptz,
    terminal_at timestamptz,
    suppressed_silence_id uuid REFERENCES silences(id) ON DELETE SET NULL,
    last_error_code text CHECK (
        last_error_code IS NULL OR last_error_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (is_test AND event_kind = 'test' AND incident_event_id IS NULL AND incident_id IS NULL)
        OR
        (NOT is_test AND event_kind <> 'test' AND incident_event_id IS NOT NULL AND incident_id IS NOT NULL)
    ),
    CHECK ((state = 'suppressed') = (suppressed_silence_id IS NOT NULL)),
    CHECK (delivered_at IS NULL OR state = 'delivered'),
    CHECK (terminal_at IS NULL OR state IN ('delivered', 'failed', 'dead_letter', 'suppressed')),
    CHECK ((state = 'attempting') = (claim_token IS NOT NULL AND claimed_until IS NOT NULL))
);

CREATE UNIQUE INDEX notification_intents_event_destination_idx
    ON notification_intents (incident_event_id, destination_id)
    WHERE incident_event_id IS NOT NULL;
CREATE INDEX notification_intents_due_idx
    ON notification_intents (available_at, created_at, id)
    WHERE state IN ('queued', 'retry_wait', 'attempting');
CREATE INDEX notification_intents_incident_idx
    ON notification_intents (incident_id, created_at DESC, id DESC)
    WHERE incident_id IS NOT NULL;
CREATE INDEX notification_intents_destination_idx
    ON notification_intents (destination_id, created_at DESC, id DESC);

CREATE TABLE notification_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    intent_id uuid NOT NULL REFERENCES notification_intents(id) ON DELETE CASCADE,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    outcome text NOT NULL CHECK (outcome IN ('delivered', 'retry', 'failed', 'dead_letter')),
    http_status integer CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    safe_error_code text CHECK (
        safe_error_code IS NULL OR safe_error_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    provider_request_id text CHECK (
        provider_request_id IS NULL OR length(provider_request_id) BETWEEN 1 AND 128
    ),
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,
    duration_ms integer NOT NULL CHECK (duration_ms BETWEEN 0 AND 3600000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (intent_id, attempt_number),
    CHECK (completed_at >= started_at)
);

CREATE INDEX notification_attempts_intent_idx
    ON notification_attempts (intent_id, attempt_number DESC);

CREATE OR REPLACE FUNCTION reject_notification_attempt_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'notification_attempts is append-only' USING ERRCODE = '42501';
END;
$$;

CREATE TRIGGER notification_attempts_reject_update
BEFORE UPDATE ON notification_attempts
FOR EACH STATEMENT EXECUTE FUNCTION reject_notification_attempt_mutation();

CREATE TRIGGER notification_attempts_reject_delete
BEFORE DELETE ON notification_attempts
FOR EACH STATEMENT EXECUTE FUNCTION reject_notification_attempt_mutation();

ALTER TABLE administrative_mutation_idempotency
    DROP CONSTRAINT administrative_mutation_idempotency_target_type_check,
    DROP CONSTRAINT administrative_mutation_idempotency_operation_check;
ALTER TABLE administrative_mutation_idempotency
    ADD CONSTRAINT administrative_mutation_idempotency_target_type_check
        CHECK (target_type IN (
            'incident_rule', 'maintenance_window', 'silence', 'notification_destination'
        )),
    ADD CONSTRAINT administrative_mutation_idempotency_operation_check
        CHECK (operation IN ('create', 'replace', 'revoke', 'test'));
