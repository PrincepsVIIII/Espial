CREATE TABLE integrations (
    id uuid PRIMARY KEY,
    adapter_id text NOT NULL,
    display_name text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    config_nonsecret jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_references jsonb NOT NULL DEFAULT '{}'::jsonb,
    interval_seconds integer NOT NULL DEFAULT 300 CHECK (interval_seconds BETWEEN 1 AND 86400),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE adapter_instances (
    id uuid PRIMARY KEY,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    adapter_version text NOT NULL,
    state text NOT NULL CHECK (state IN ('starting', 'healthy', 'unhealthy', 'stopped')),
    last_started_at timestamptz,
    last_healthy_at timestamptz,
    last_error_code text,
    UNIQUE (integration_id)
);

CREATE TABLE resources (
    id uuid PRIMARY KEY,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    external_id text NOT NULL,
    kind text NOT NULL,
    display_name text NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    source_url text,
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    UNIQUE (integration_id, external_id)
);

CREATE INDEX resources_kind_idx ON resources (kind);

CREATE TABLE observations (
    id uuid PRIMARY KEY,
    resource_id uuid NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    check_type text NOT NULL,
    observed_state text NOT NULL CHECK (
        observed_state IN ('healthy', 'warning', 'critical', 'unknown', 'maintenance', 'disabled')
    ),
    summary text NOT NULL,
    observed_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    measurements jsonb NOT NULL DEFAULT '{}'::jsonb,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX observations_resource_observed_idx
    ON observations (resource_id, observed_at DESC);
CREATE INDEX observations_received_at_idx ON observations (received_at);

CREATE TABLE current_health (
    resource_id uuid PRIMARY KEY REFERENCES resources(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (
        state IN ('healthy', 'warning', 'critical', 'unknown', 'stale', 'maintenance', 'disabled')
    ),
    reason text NOT NULL,
    observation_id uuid REFERENCES observations(id) ON DELETE SET NULL,
    observed_at timestamptz,
    last_success_at timestamptz,
    stale_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX current_health_state_idx ON current_health (state);
CREATE INDEX current_health_stale_at_idx ON current_health (stale_at);
