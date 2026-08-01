# Architecture decision records

Status values are `Proposed`, `Accepted`, `Superseded`, or `Deprecated`. ADRs are
append-only records; supersede an accepted decision rather than rewriting its
history.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-go-core.md) | Go for Espial Core | Accepted |
| [0002](0002-postgresql.md) | PostgreSQL as the operational store | Accepted |
| [0003](0003-stdio-adapter-transport.md) | NDJSON over supervised stdio for the first adapter transport | Accepted |
| [0004](0004-server-sent-events.md) | Server-sent events for initial live updates | Accepted |
| [0005](0005-initial-deployment.md) | Single-host container deployment with separate processes | Accepted |
| [0006](0006-authentication-transition.md) | Local authentication now, provider-based SSO transition later | Accepted |
| [0007](0007-durable-monitoring-signal-journal.md) | Durable PostgreSQL monitoring-signal journal for incident evaluation | Accepted |
| [0008](0008-durable-notification-outbox.md) | Durable notification outbox and in-process Mattermost driver | Accepted |
| [0009](0009-website-monitor-integration-projection.md) | Website monitors are typed integration projections | Accepted |
