CREATE TABLE users (
    id uuid PRIMARY KEY,
    username text NOT NULL,
    display_name text NOT NULL,
    email text,
    identity_provider text NOT NULL,
    external_subject text,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (identity_provider, external_subject)
);

CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));

CREATE TABLE local_credentials (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash text NOT NULL,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    locked_until timestamptz
);

CREATE TABLE roles (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    permissions jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL UNIQUE,
    csrf_digest bytea NOT NULL,
    source_address inet,
    last_seen_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at <= absolute_expires_at)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

INSERT INTO roles (id, name, permissions) VALUES
    ('10000000-0000-4000-8000-000000000001', 'viewer',
     '["overview:read", "resources:read", "integrations:read"]'::jsonb),
    ('10000000-0000-4000-8000-000000000002', 'operator',
     '["overview:read", "resources:read", "integrations:read", "incidents:operate"]'::jsonb),
    ('10000000-0000-4000-8000-000000000003', 'administrator',
     '["overview:read", "resources:read", "integrations:read", "audit:read", "incidents:operate", "integrations:manage", "users:manage", "roles:manage"]'::jsonb);
