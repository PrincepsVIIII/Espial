ALTER TABLE incident_rules
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE monitoring_signals DROP CONSTRAINT monitoring_signals_kind_check;
ALTER TABLE monitoring_signals ADD CONSTRAINT monitoring_signals_kind_check
    CHECK (kind IN ('observation', 'freshness', 'maintenance_expiry'));

CREATE TABLE maintenance_windows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 512),
    integration_id uuid REFERENCES integrations(id) ON DELETE CASCADE,
    resource_id uuid REFERENCES resources(id) ON DELETE CASCADE,
    check_type text CHECK (
        check_type IS NULL OR check_type ~ '^[a-z][a-z0-9_.-]{0,126}$'
    ),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    revoked_at timestamptz,
    expired_at timestamptz,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by_name text NOT NULL CHECK (length(created_by_name) BETWEEN 1 AND 128),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (integration_id IS NOT NULL OR resource_id IS NOT NULL OR check_type IS NOT NULL),
    CHECK (ends_at > starts_at),
    CHECK (ends_at <= starts_at + interval '366 days'),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CHECK (expired_at IS NULL OR expired_at >= starts_at)
);

CREATE INDEX maintenance_windows_active_idx
    ON maintenance_windows (starts_at, ends_at, id)
    WHERE enabled AND revoked_at IS NULL;
CREATE INDEX maintenance_windows_expiry_idx
    ON maintenance_windows (ends_at, id)
    WHERE expired_at IS NULL;
CREATE INDEX maintenance_windows_resource_idx
    ON maintenance_windows (resource_id, starts_at, ends_at)
    WHERE enabled AND revoked_at IS NULL;
CREATE INDEX maintenance_windows_integration_idx
    ON maintenance_windows (integration_id, starts_at, ends_at)
    WHERE enabled AND revoked_at IS NULL;

CREATE TABLE silences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 512),
    incident_id uuid REFERENCES incidents(id) ON DELETE CASCADE,
    rule_id uuid REFERENCES incident_rules(id) ON DELETE CASCADE,
    resource_id uuid REFERENCES resources(id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    revoked_at timestamptz,
    expired_at timestamptz,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by_name text NOT NULL CHECK (length(created_by_name) BETWEEN 1 AND 128),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(incident_id, rule_id, resource_id) = 1),
    CHECK (expires_at > starts_at),
    CHECK (expires_at <= starts_at + interval '366 days'),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CHECK (expired_at IS NULL OR expired_at >= starts_at)
);

CREATE INDEX silences_active_incident_idx
    ON silences (incident_id, starts_at, expires_at)
    WHERE enabled AND revoked_at IS NULL;
CREATE INDEX silences_active_rule_idx
    ON silences (rule_id, starts_at, expires_at)
    WHERE enabled AND revoked_at IS NULL;
CREATE INDEX silences_active_resource_idx
    ON silences (resource_id, starts_at, expires_at)
    WHERE enabled AND revoked_at IS NULL;
CREATE INDEX silences_expiry_idx
    ON silences (expires_at, id) WHERE expired_at IS NULL;

CREATE TABLE administrative_mutation_idempotency (
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type text NOT NULL CHECK (target_type IN ('incident_rule', 'maintenance_window', 'silence')),
    operation text NOT NULL CHECK (operation IN ('create', 'replace', 'revoke')),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    request_hash text NOT NULL CHECK (length(request_hash) = 64),
    target_id uuid NOT NULL,
    result_version bigint NOT NULL CHECK (result_version > 0),
    correlation_id text NOT NULL CHECK (length(correlation_id) BETWEEN 1 AND 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_user_id, target_type, operation, idempotency_key)
);

CREATE TABLE incident_evaluation_evidence (
    signal_id uuid PRIMARY KEY REFERENCES monitoring_signals(id) ON DELETE CASCADE,
    rule_id uuid REFERENCES incident_rules(id) ON DELETE SET NULL,
    maintenance_window_id uuid REFERENCES maintenance_windows(id) ON DELETE SET NULL,
    outcome text NOT NULL CHECK (
        outcome IN ('no_rule', 'no_condition', 'debounced', 'incident_changed', 'maintenance')
    ),
    explanation text NOT NULL CHECK (length(explanation) BETWEEN 1 AND 1024),
    evaluated_at timestamptz NOT NULL
);

CREATE INDEX incident_evaluation_evidence_rule_idx
    ON incident_evaluation_evidence (rule_id, evaluated_at DESC);
CREATE INDEX incident_evaluation_evidence_maintenance_idx
    ON incident_evaluation_evidence (maintenance_window_id, evaluated_at DESC)
    WHERE maintenance_window_id IS NOT NULL;
