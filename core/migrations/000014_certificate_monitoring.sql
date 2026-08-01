CREATE TABLE certificate_observations (
    observation_id uuid PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    endpoint text NOT NULL CHECK (length(endpoint) BETWEEN 1 AND 512),
    subject_summary text CHECK (subject_summary IS NULL OR length(subject_summary) <= 512),
    san_summary text CHECK (san_summary IS NULL OR length(san_summary) <= 1024),
    issuer_summary text CHECK (issuer_summary IS NULL OR length(issuer_summary) <= 512),
    serial_number text CHECK (serial_number IS NULL OR length(serial_number) <= 128),
    fingerprint_sha256 text CHECK (fingerprint_sha256 IS NULL OR fingerprint_sha256 ~ '^[0-9a-f]{64}$'),
    not_before timestamptz,
    not_after timestamptz,
    hostname_valid boolean,
    chain_valid boolean,
    days_remaining integer,
    certificate_state text NOT NULL CHECK (certificate_state IN ('healthy', 'warning', 'critical', 'unknown')),
    reason_code text NOT NULL CHECK (reason_code ~ '^[a-z][a-z0-9_.-]{0,126}$'),
    fingerprint_changed boolean NOT NULL DEFAULT false,
    issuer_changed boolean NOT NULL DEFAULT false,
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (not_before IS NULL OR not_after IS NULL OR not_after > not_before)
);

CREATE INDEX certificate_observations_latest_idx
    ON certificate_observations (resource_id, observed_at DESC, observation_id DESC);
CREATE INDEX certificate_observations_expiry_idx
    ON certificate_observations (not_after, resource_id)
    WHERE not_after IS NOT NULL;
CREATE INDEX certificate_observations_attention_idx
    ON certificate_observations (certificate_state, observed_at DESC, resource_id)
    WHERE certificate_state IN ('warning', 'critical');

INSERT INTO incident_rules (
    id, name, enabled, priority, resource_kind, check_type, recovery_state,
    recovery_min_occurrences, recovery_for_seconds
) VALUES (
    '20000000-0000-4000-8000-000000000003',
    'Certificate validity and expiry', true, 250, 'certificate',
    'certificate.validity', 'healthy', 2, 0
);

INSERT INTO incident_rule_conditions (
    rule_id, state, severity, min_occurrences, for_seconds
) VALUES
    ('20000000-0000-4000-8000-000000000003', 'critical', 'critical', 1, 0),
    ('20000000-0000-4000-8000-000000000003', 'warning', 'warning', 1, 0),
    ('20000000-0000-4000-8000-000000000003', 'unknown', 'warning', 1, 0);

ALTER TABLE incident_rule_states ADD COLUMN last_reason_code text CHECK (
    last_reason_code IS NULL OR last_reason_code ~ '^[a-z][a-z0-9_.-]{0,126}$'
);

ALTER TABLE incident_timeline DROP CONSTRAINT incident_timeline_kind_check;
ALTER TABLE incident_timeline ADD CONSTRAINT incident_timeline_kind_check CHECK (
    kind IN (
        'detected', 'severity_changed', 'condition_changed', 'recurrence', 'recovered',
        'acknowledged', 'investigating', 'assigned', 'note', 'resolved', 'notification'
    )
);

ALTER TABLE notification_intents DROP CONSTRAINT notification_intents_event_kind_check;
ALTER TABLE notification_intents ADD CONSTRAINT notification_intents_event_kind_check CHECK (
    event_kind IN ('detected', 'severity_changed', 'condition_changed', 'recurrence', 'recovered', 'test')
);
