# Logs and troubleshooting

## First checks

Check Compose service health and Core `/api/v1/health/live` and
`/api/v1/health/ready`. Liveness proves the process is running; readiness also
requires PostgreSQL and a compatible migrated schema. Use `docker compose logs
--since` for the smallest useful window. Production JSON logs rotate at 10 MiB with
five files per container.

Logs are structured and must remain free of passwords, session/CSRF tokens, DSNs,
adapter secrets, and raw secret values. Request IDs connect HTTP failures to audit
events. Do not paste unredacted logs into tickets.

## Adapter diagnosis

Inspect integration health, lifecycle audit actions, `last_error_code`, restart
timing, last successful collection, and resource freshness. Error codes are stable
categories; arbitrary child stderr is bounded and redacted. A crashing, hanging,
flooding, malformed, or protocol-incompatible adapter must degrade its integration
and eventually make data stale/unknown without making Core unready.

Before re-enabling an integration, fix its executable/config/secret reference and
confirm its egress policy. Disable/re-enable through the audited lifecycle API; do
not edit database state. Repeated failure uses bounded exponential backoff. Core
shutdown during backoff persists `stopped`, so an old `running` value after restart
is a defect.

## Login failures

Confirm the browser origin exactly matches `ESPIAL_PUBLIC_URL`, including scheme and
port. “Request origin not accepted” is a CSRF defense, normally caused by a stale or
incorrect public URL. Restart Core after correcting it. Generic credential failures
may also mean lockout or a disabled account; use the audited recovery commands in
[LOCAL_AUTH.md](LOCAL_AUTH.md), not direct SQL.

## Database/readiness failures

Confirm secret file permissions, DSN role, PostgreSQL health, connection limits,
and that the one-shot migration completed. Runtime Core intentionally cannot change
existing audit records and must not run owner migrations. Stop a rollout on schema
drift or an audit-write failure; protected administrative mutations fail closed.

Readiness also requires required workers to have initialized. If PostgreSQL is
healthy but readiness never succeeds, inspect the first Core runtime error rather
than restarting in a loop. A Mattermost or monitored website outage does not make
Core unready.

## Phase 2 backlog

Read private Core `/metrics` and compare signal queue depth/oldest age,
notification states/oldest due age, active incidents/controls, and webpage/
certificate state counts. Labels are intentionally fixed and never identify a
resource. Follow [notification recovery](NOTIFICATION_RECOVERY.md) for retry or
dead-letter state and [incident operations](INCIDENT_AND_RULE_OPERATIONS.md) for
rule/suppression behavior. Do not change queue rows with SQL or raise concurrency
before ruling out database saturation, poison work, egress policy, and provider
failure.
