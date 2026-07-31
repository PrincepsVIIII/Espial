# Initial Phase 1 data model

PostgreSQL is authoritative for identity, configuration, current health, normalized
observations, and audit history. The diagram is intentionally limited to Phase 1;
incident, notification, certificate, dependency, and physical inventory tables are
added in their roadmap phases.

```mermaid
erDiagram
    USERS ||--o{ USER_ROLES : has
    ROLES ||--o{ USER_ROLES : grants
    USERS ||--o| LOCAL_CREDENTIALS : authenticates
    USERS ||--o{ SESSIONS : opens
    USERS ||--o{ AUDIT_EVENTS : acts
    INTEGRATIONS ||--o{ ADAPTER_INSTANCES : runs
    INTEGRATIONS ||--o{ RESOURCES : discovers
    RESOURCES ||--o{ OBSERVATIONS : receives
    RESOURCES ||--|| CURRENT_HEALTH : summarizes
    INTEGRATIONS ||--o{ AUDIT_EVENTS : targets

    USERS {
      uuid id PK
      text username UK
      text display_name
      text email
      text identity_provider
      text external_subject
      boolean enabled
      timestamptz created_at
      timestamptz updated_at
    }
    LOCAL_CREDENTIALS {
      uuid user_id PK,FK
      text password_hash
      timestamptz password_changed_at
      integer failed_attempts
      timestamptz locked_until
    }
    ROLES {
      uuid id PK
      text name UK
      jsonb permissions
    }
    USER_ROLES {
      uuid user_id PK,FK
      uuid role_id PK,FK
    }
    SESSIONS {
      uuid id PK
      uuid user_id FK
      bytea token_digest UK
      bytea csrf_digest
      inet source_address
      timestamptz last_seen_at
      timestamptz expires_at
      timestamptz revoked_at
    }
    INTEGRATIONS {
      uuid id PK
      text adapter_id
      text display_name
      boolean enabled
      jsonb config_nonsecret
      jsonb secret_references
      integer interval_seconds
      timestamptz created_at
      timestamptz updated_at
    }
    ADAPTER_INSTANCES {
      uuid id PK
      uuid integration_id FK
      text adapter_version
      text protocol_version
      text state
      timestamptz last_started_at
      timestamptz last_healthy_at
      timestamptz last_stopped_at
      timestamptz last_error_at
      text last_error_code
      integer consecutive_failures
      timestamptz next_restart_at
      timestamptz updated_at
    }
    RESOURCES {
      uuid id PK
      uuid integration_id FK
      text external_id
      text kind
      text display_name
      jsonb attributes
      timestamptz first_seen_at
      timestamptz last_seen_at
    }
    OBSERVATIONS {
      uuid id PK
      uuid resource_id FK
      uuid integration_id FK
      text check_type
      text observed_state
      text summary
      timestamptz observed_at
      timestamptz received_at
      integer expected_refresh_seconds
      timestamptz expires_at
      jsonb measurements
      jsonb metadata
    }
    CURRENT_HEALTH {
      uuid resource_id PK,FK
      text state
      text reason
      uuid observation_id FK
      timestamptz observed_at
      timestamptz last_success_at
      timestamptz stale_at
      timestamptz unknown_at
      timestamptz updated_at
    }
    AUDIT_EVENTS {
      uuid id PK
      uuid actor_user_id FK
      text action
      text target_type
      text target_id
      text result
      inet source_address
      text correlation_id
      jsonb before_summary
      jsonb after_summary
      timestamptz occurred_at
    }
```

## Invariants

- `(integration_id, external_id)` uniquely identifies a resource.
- `current_health` has exactly one row per resource and references the observation
  that last changed its evaluated state.
- Observation ingestion and current-state replacement happen in one transaction.
- `observed_at` is source time; ingestion time is separately recorded in migrations
  even when omitted from the conceptual diagram.
- `(integration_id, resource_id, check_type, observed_at)` is the normalized
  observation delivery key when a source UUID is absent. Retrying the same payload
  is a no-op; conflicting content for the same key is rejected.
- Staleness is derived from expected refresh and persisted as a state transition so
  clients receive a live event and history remains explainable.
- Audit records are append-only to application roles. Corrections create a new
  event; they do not update old events.
- Secrets are references, never values, in `integrations` and audit summaries.
- Adapter-instance restart counters and deadlines are persisted so restarting Core
  cannot erase a failing adapter's backoff. Host-local process IDs are not stored.

## Migration rules

Migrations are numbered, forward-only SQL with `up` and explicit documented
rollback/recovery guidance. Core refuses to start when the database is newer than
the binary. Destructive or long-running migrations require a two-release expand /
migrate / contract sequence.

## Retention in Phase 1

Keep current state indefinitely while the resource exists. Retain detailed
observations for 90 days initially and audit events for at least six months; make
both durations configurable but enforce a safe minimum for audit data. A later
retention worker will downsample older operational measurements rather than copy a
metrics platform.
