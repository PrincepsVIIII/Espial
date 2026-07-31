CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text,
    result text NOT NULL CHECK (result IN ('succeeded', 'failed', 'denied')),
    source_address inet,
    correlation_id text NOT NULL,
    before_summary jsonb,
    after_summary jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_occurred_at_idx ON audit_events (occurred_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, occurred_at DESC);
CREATE INDEX audit_events_target_idx ON audit_events (target_type, target_id, occurred_at DESC);
