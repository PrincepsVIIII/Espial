# Adapter protocol v1

Phase 1 uses supervised newline-delimited JSON (NDJSON) over standard input/output.
The logical operations can later be carried over local HTTP without changing their
models.

## Process contract

1. Core starts the configured executable with a minimal environment and dedicated
   working directory.
2. Adapter writes exactly one `ready` notification after initialization.
3. Core sends request envelopes on stdin; the adapter returns one terminal response
   with the same `request_id` on stdout.
4. Either side may send supported notifications. Stderr is captured as bounded,
   redacted diagnostic output.
5. On shutdown, Core sends `shutdown`, stops scheduling new work, waits for the
   deadline, then terminates the process if necessary.

Each stdout line is one JSON object no larger than 1 MiB. Invalid JSON, unknown
protocol major versions, duplicate terminal responses, output floods, and missed
deadlines are protocol failures. Core records them and applies bounded restart
backoff.

## Envelope

```json
{
  "protocol_version": "1.0",
  "kind": "request",
  "operation": "collect",
  "request_id": "b8ab8b5b-4466-45af-b285-05f0643dc024",
  "sent_at": "2026-07-30T15:00:00Z",
  "deadline": "2026-07-30T15:00:30Z",
  "payload": {}
}
```

Responses use `kind: response`, repeat `operation` and `request_id`, and include
either `payload` or a safe structured `error`. Notifications use
`kind: notification` and omit `request_id` and `deadline`.

## Operations

| Operation | Direction | Purpose |
|---|---|---|
| `manifest` | Core → adapter | Return identity, configuration schema, and capabilities |
| `validate_config` | Core → adapter | Validate redacted/resolved configuration before activation |
| `collect` | Core → adapter | Produce a bounded batch of normalized resources and observations |
| `health` | Core → adapter | Report adapter readiness, not monitored-resource health |
| `shutdown` | Core → adapter | Graceful process termination |
| `ready` | adapter → Core notification | Initialization complete |
| `event` | adapter → Core notification | Optional event-driven normalized input |
| `log` | adapter → Core notification | Structured, bounded diagnostic record |

The first sample adapter must implement `manifest`, `validate_config`, `collect`,
`health`, and `shutdown`. Event subscription is optional in the first slice.

## Collection payload

A successful `collect` response contains `resources`, `observations`, and an
optional opaque `next_cursor`. Every record is validated against schemas before a
transaction begins. Core rejects the entire batch on structural errors; domain
validation errors identify record indexes without echoing secrets.

Configuration payloads distinguish ordinary values from secret references. Core
resolves a reference only for the child process and never returns resolved secrets
through APIs, audit diffs, protocol errors, or logs.

## Supervision and stale behavior

- One collection is in flight per integration unless its manifest declares a safe
  higher limit.
- Default collection timeout is 30 seconds and is operator-configurable within
  bounds.
- Restart delays use exponential backoff with jitter and a cap; a healthy interval
  resets the failure counter.
- Core marks current data `stale` after its expected refresh window plus grace. It
  becomes `unknown` after a configurable multiple or when no valid observation has
  ever arrived.
- Process health never overwrites resource health. A failed adapter explains why
  dependent observations are stale/unknown.

## Versioning and conformance

Major protocol versions are incompatible; minor versions are additive. The Core
negotiates the highest mutually supported minor version after reading the manifest.
Golden NDJSON fixtures and a conformance harness will test happy path, malformed
lines, oversized messages, timeout, crash/restart, shutdown, and unknown fields.

Machine-readable definitions are indexed in [`api/README.md`](../../api/README.md).
