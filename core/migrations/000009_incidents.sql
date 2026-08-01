CREATE TABLE incident_rules (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    enabled boolean NOT NULL DEFAULT true,
    priority integer NOT NULL DEFAULT 100 CHECK (priority BETWEEN 0 AND 10000),
    integration_id uuid REFERENCES integrations(id) ON DELETE CASCADE,
    resource_id uuid REFERENCES resources(id) ON DELETE CASCADE,
    resource_kind text CHECK (
        resource_kind IS NULL OR resource_kind ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    check_type text CHECK (
        check_type IS NULL OR check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    reason_code text CHECK (
        reason_code IS NULL OR reason_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    recovery_state text NOT NULL DEFAULT 'healthy' CHECK (
        recovery_state IN ('healthy', 'warning', 'critical', 'unknown', 'stale', 'maintenance', 'disabled')
    ),
    recovery_min_occurrences integer NOT NULL DEFAULT 2
        CHECK (recovery_min_occurrences BETWEEN 1 AND 1000),
    recovery_for_seconds integer NOT NULL DEFAULT 0
        CHECK (recovery_for_seconds BETWEEN 0 AND 2592000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE incident_rule_conditions (
    rule_id uuid NOT NULL REFERENCES incident_rules(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('warning', 'critical', 'unknown', 'stale')),
    severity text NOT NULL CHECK (severity IN ('warning', 'critical')),
    min_occurrences integer NOT NULL DEFAULT 1 CHECK (min_occurrences BETWEEN 1 AND 1000),
    for_seconds integer NOT NULL DEFAULT 0 CHECK (for_seconds BETWEEN 0 AND 2592000),
    PRIMARY KEY (rule_id, state)
);

CREATE TABLE incidents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id uuid NOT NULL REFERENCES incident_rules(id) ON DELETE RESTRICT,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE RESTRICT,
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE RESTRICT,
    check_type text NOT NULL CHECK (check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'),
    fingerprint text NOT NULL CHECK (length(fingerprint) BETWEEN 1 AND 256),
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 256),
    summary text NOT NULL CHECK (length(summary) BETWEEN 1 AND 1024),
    severity text NOT NULL CHECK (severity IN ('warning', 'critical')),
    status text NOT NULL CHECK (
        status IN ('open', 'acknowledged', 'investigating', 'recovered', 'resolved')
    ),
    owner_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    detected_at timestamptz NOT NULL,
    latest_signal_at timestamptz NOT NULL,
    acknowledged_at timestamptz,
    recovered_at timestamptz,
    resolved_at timestamptz,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (latest_signal_at >= detected_at),
    CHECK (recovered_at IS NULL OR recovered_at >= detected_at),
    CHECK (resolved_at IS NULL OR recovered_at IS NOT NULL AND resolved_at >= recovered_at)
);

CREATE UNIQUE INDEX incidents_active_fingerprint_idx
    ON incidents (fingerprint) WHERE status <> 'resolved';
CREATE INDEX incidents_active_order_idx
    ON incidents (updated_at DESC, id DESC) WHERE status NOT IN ('recovered', 'resolved');
CREATE INDEX incidents_history_order_idx
    ON incidents (updated_at DESC, id DESC) WHERE status IN ('recovered', 'resolved');
CREATE INDEX incidents_resource_idx
    ON incidents (resource_id, updated_at DESC, id DESC);

CREATE TABLE incident_timeline (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    signal_id uuid REFERENCES monitoring_signals(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    kind text NOT NULL CHECK (
        kind IN (
            'detected', 'severity_changed', 'recurrence', 'recovered',
            'acknowledged', 'investigating', 'assigned', 'note', 'resolved',
            'notification'
        )
    ),
    from_status text,
    to_status text,
    from_severity text,
    to_severity text,
    summary text NOT NULL CHECK (length(summary) BETWEEN 1 AND 2048),
    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX incident_timeline_incident_time_idx
    ON incident_timeline (incident_id, occurred_at DESC, id DESC);

CREATE OR REPLACE FUNCTION reject_incident_timeline_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'incident_timeline is append-only' USING ERRCODE = '42501';
END;
$$;

CREATE TRIGGER incident_timeline_reject_update
BEFORE UPDATE ON incident_timeline
FOR EACH STATEMENT EXECUTE FUNCTION reject_incident_timeline_mutation();

CREATE TRIGGER incident_timeline_reject_delete
BEFORE DELETE ON incident_timeline
FOR EACH STATEMENT EXECUTE FUNCTION reject_incident_timeline_mutation();

CREATE TABLE incident_rule_states (
    rule_id uuid NOT NULL REFERENCES incident_rules(id) ON DELETE CASCADE,
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    check_type text NOT NULL CHECK (check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'),
    active_incident_id uuid REFERENCES incidents(id) ON DELETE SET NULL,
    last_signal_id uuid REFERENCES monitoring_signals(id) ON DELETE SET NULL,
    last_signal_at timestamptz,
    last_state text CHECK (
        last_state IS NULL OR last_state IN ('healthy', 'warning', 'critical', 'unknown', 'stale', 'maintenance', 'disabled')
    ),
    last_reason text,
    matching_since timestamptz,
    matching_occurrences integer NOT NULL DEFAULT 0 CHECK (matching_occurrences >= 0),
    recovery_since timestamptz,
    recovery_occurrences integer NOT NULL DEFAULT 0 CHECK (recovery_occurrences >= 0),
    deadline_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (rule_id, resource_id, check_type)
);

CREATE INDEX incident_rule_states_due_idx
    ON incident_rule_states (deadline_at, rule_id, resource_id, check_type)
    WHERE deadline_at IS NOT NULL;

INSERT INTO incident_rules (
    id, name, enabled, priority, recovery_state, recovery_min_occurrences,
    recovery_for_seconds
) VALUES (
    '20000000-0000-4000-8000-000000000001',
    'Default resource health', true, 100, 'healthy', 2, 0
);

INSERT INTO incident_rule_conditions (
    rule_id, state, severity, min_occurrences, for_seconds
) VALUES
    ('20000000-0000-4000-8000-000000000001', 'critical', 'critical', 1, 0),
    ('20000000-0000-4000-8000-000000000001', 'warning', 'warning', 2, 0),
    ('20000000-0000-4000-8000-000000000001', 'unknown', 'warning', 1, 0);

