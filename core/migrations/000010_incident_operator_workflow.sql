-- Slice 2.2 keeps operator evidence immutable and makes retries durable.
ALTER TABLE incident_timeline
    ADD COLUMN actor_display_name text CHECK (
        actor_display_name IS NULL OR length(actor_display_name) BETWEEN 1 AND 128
    ),
    ADD COLUMN subject_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN subject_display_name text CHECK (
        subject_display_name IS NULL OR length(subject_display_name) BETWEEN 1 AND 128
    );

-- Existing automated events have no actor. Future operator events always snapshot
-- display text so later account renames or deletion cannot rewrite the evidence.
CREATE TABLE incident_action_idempotency (
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    incident_id uuid NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    action text NOT NULL CHECK (
        action IN ('acknowledge', 'investigate', 'assign', 'note', 'resolve')
    ),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    result_version bigint NOT NULL CHECK (result_version > 0),
    result_status text NOT NULL CHECK (
        result_status IN ('open', 'acknowledged', 'investigating', 'recovered', 'resolved')
    ),
    timeline_event_id uuid NOT NULL REFERENCES incident_timeline(id) ON DELETE RESTRICT,
    correlation_id text NOT NULL CHECK (length(correlation_id) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_user_id, incident_id, action, idempotency_key)
);

CREATE INDEX incident_action_idempotency_created_idx
    ON incident_action_idempotency (created_at);
