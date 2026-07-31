# Slice 1.4 execution plan: adapter runtime and sample adapter

**Status:** Implemented 2026-07-31  
**Parent:** [Phase 1 monitoring foundation](PHASE_1_IMPLEMENTATION.md)  
**Depends on:** [Slice 1.3 normalized storage and health](SLICE_1_3_NORMALIZED_HEALTH.md)  
**Contracts:** [adapter protocol](../architecture/ADAPTER_PROTOCOL.md),
[adapter envelope](../../api/schemas/v1/adapter-envelope.schema.json),
[adapter manifest](../../api/schemas/v1/adapter-manifest.schema.json), and
[stdio transport ADR](../adr/0003-stdio-adapter-transport.md)

## Outcome

Slice 1.4 creates the isolated runtime boundary between Core and locally supervised
adapter processes. Core can register an explicitly trusted executable, start one
process for an integration, negotiate protocol v1, exchange bounded correlated
requests, record safe process health, stop and reap the child, and restart failures
without a tight loop. A deterministic sample adapter and black-box conformance
harness prove the contract without real infrastructure or credentials.

This slice does not schedule collections, ingest returned batches, run freshness
deadlines, expose adapter APIs, or deliver live browser events. Slice 1.5 connects a
successful `collect` payload to the Slice 1.3 observation service and owns scheduled
freshness reevaluation.

## Readiness findings

Available now:

- accepted NDJSON stdio transport and v1 envelope/manifest schemas;
- integration and adapter-instance tables with non-secret configuration separated
  from secret references;
- Slice 1.3 transport-independent resource and observation inputs;
- Core lifecycle, injected configuration patterns, structured logs, PostgreSQL,
  and graceful cancellation; and
- a documented deterministic sample integration that requires no production
  inventory.

The implementation adds migration
`000005_adapter_runtime.sql` adds the negotiated protocol version, last stop/error
times, consecutive failure count, next permitted restart time, and runtime update
time to `adapter_instances`. Process IDs are deliberately not persisted because
they are host-local and unsafe to reuse after Core restarts.

No SSO decision, production adapter, secret-store selection, hostname, or vendor
credential blocks this slice. The sample adapter has no secrets. A resolver
interface and fake test resolver establish the future secret boundary without
pretending that the production store has been selected.

## Accepted runtime semantics

### Registry and executable trust

- A registry entry is an immutable Core-wiring descriptor containing adapter ID,
  exact executable path, argument list, working directory, and permitted static
  environment keys.
- Integration rows select a registered adapter by ID. Database configuration and
  HTTP input can never supply an executable path, arbitrary arguments, shell text,
  or working directory.
- Core starts the executable directly with `exec.CommandContext`; it never invokes
  a shell. Runtime integration config and resolved secrets travel only in bounded
  stdin request payloads, never command arguments or environment variables.
- The initial application registry contains only the sample adapter. Tests inject
  explicit fixture descriptors. A future external-adapter installation mechanism
  requires its own reviewed trust and filesystem boundary.

### Startup and version negotiation

1. Start the child with owned stdin/stdout/stderr pipes and one wait/reap owner.
2. Require exactly one `ready` notification within 10 seconds. Any earlier response,
   non-protocol stdout, duplicate ready, or unsupported major is a protocol failure.
3. Send `manifest` using Core's bootstrap version `1.0` and validate the response
   structurally and semantically.
4. Require the manifest adapter ID to equal the trusted registry ID and require the
   capabilities used by the integration.
5. Select the highest exact version advertised by both sides. Slice 1.4 Core
   supports `1.0`; a manifest may advertise additive later minors alongside `1.0`,
   but no assumed minor downgrade occurs when the exact common version is absent.
6. Persist the adapter version and negotiated protocol version, then run
   `validate_config` and `health`. The process becomes healthy only after all three
   exchanges succeed.

Unknown major versions fail with `unsupported_protocol_major`; a v1 manifest with
no exact common minor fails with `unsupported_protocol_minor`. These are safe,
stable codes, not raw parser errors.

### Envelope and request rules

- Decode one complete line at a time with a hard 1 MiB encoded limit before JSON
  allocation. EOF with a partial line is invalid.
- Use strict envelope decoding: unknown envelope fields, invalid timestamps/UUIDs,
  invalid kind/operation combinations, or both/neither payload and error where a
  terminal response requires one are rejected.
- Permit one request in flight per integration in Slice 1.4. Generate the request
  UUID in Core, copy one captured sent/deadline time, and require the response's
  request ID and operation to match.
- Exactly one terminal response is allowed. A duplicate, unmatched response,
  response after deadline, response after cancellation, or payload following a
  terminal error fails that process generation.
- Only `ready` and bounded `log` notifications are consumed in this slice. `event`
  input is enabled when Slice 1.5 can validate and ingest it; an adapter claiming
  or sending unsupported notification behavior fails clearly rather than silently
  dropping operational data.
- A failed exchange returns a typed category/code without returning request,
  configuration, secret, stderr, or adapter-provided payload text.

### Configuration and secrets

- `config_nonsecret` and `secret_references` remain separate at rest and in domain
  types. A secret reference is an opaque string interpreted only by a `SecretResolver`.
- Resolve references immediately before `validate_config` or another request that
  needs them. Keep resolved values in the shortest-lived request object possible;
  never persist them or include them in result/error types.
- The manifest's `secret_fields` must agree with supplied secret-reference keys.
  Missing required references, an ordinary value placed in a secret field, or a
  reference for an undeclared secret field fails configuration validation.
- The child receives resolved values over stdin. Known resolved byte sequences are
  redacted from bounded diagnostics before storage or structured logging.
- Slice 1.4 tests use a fake resolver and canary secrets. Selecting a production
  file/store resolver remains a Slice 1.8 deployment decision.

### Process health and persistence

The runtime state machine is:

```text
stopped -> starting -> healthy
              |          |
              +------> unhealthy -> starting (after backoff)
healthy/unhealthy/starting -> stopped (operator or Core shutdown)
```

- `starting` begins when Core owns a child process.
- `healthy` means ready, manifest, config validation, and adapter health completed;
  it does not describe monitored resources.
- Protocol failure, timeout, unexpected exit, or failed health changes the instance
  to `unhealthy` with a stable safe error code/time.
- Intentional disable or shutdown changes it to `stopped` and does not increment
  failure backoff.
- Each update is scoped to the integration's single adapter instance. The runtime
  repository never writes resource health.

### Restart, cancellation, and shutdown

For consecutive unexpected failures `n`, use:

```text
base_delay = min(1 second * 2^(n-1), 60 seconds)
delay      = base_delay * jitter in [0.8, 1.2]
```

- Inject the clock, timer, and jitter source so tests never sleep probabilistically.
- Persist `consecutive_failures` and `next_restart_at`; a Core restart cannot erase
  an active backoff window.
- Reset the counter after five continuous healthy minutes. A brief ready/crash loop
  therefore cannot reset its own protection.
- Cancellation stops accepting requests, sends one `shutdown` request with a
  five-second deadline when the protocol is usable, closes stdin, waits up to five
  seconds for exit, sends the platform termination signal, waits two seconds, then
  kills if required.
- Exactly one goroutine calls `Wait`. All exit paths close/drain owned pipes, resolve
  pending calls, stop timers, and publish one terminal process result.

## Initial safety and resource limits

| Boundary | Slice 1.4 limit |
|---|---:|
| Encoded stdout NDJSON line | 1 MiB |
| Startup ready deadline | 10 seconds |
| Default request deadline | 30 seconds |
| Shutdown request/process grace | 5 seconds each |
| Termination-to-kill grace | 2 seconds |
| Stored stderr/log diagnostic ring | 64 KiB per process generation |
| Individual stderr/log line retained | 16 KiB |
| Requests in flight | 1 per integration |
| Restart backoff | 1–60 seconds, ±20% jitter |
| Healthy backoff reset | 5 continuous minutes |

Readers always drain child output while retaining only bounded data. Output beyond
a protocol line limit fails the process; diagnostic overflow drops oldest retained
diagnostics and increments a counter rather than blocking the child or growing
memory.

## Package and command layout

```text
core/
├── cmd/espial-sample-adapter/  standalone deterministic reference process
├── internal/adapters/
│   ├── types.go                descriptors, manifest, safe results/errors
│   ├── registry.go             exact trusted adapter-ID lookup
│   ├── protocol.go             strict envelope/version/operation validation
│   ├── codec.go                bounded NDJSON reader/writer
│   ├── process.go              pipes, request correlation, and reap ownership
│   ├── supervisor.go           state machine, restart, stop, and cancellation
│   ├── repository.go           adapter-instance persistence only
│   ├── secrets.go              resolver boundary and redaction helpers
│   └── conformance/            reusable black-box executable harness
├── internal/sampleadapter/     sample manifest, config, fixtures, and protocol loop
└── migrations/000005_adapter_runtime.sql
```

Rules:

- `adapters` depends on normalized input types only to decode a collect result; it
  does not call the Slice 1.3 ingestion service until Slice 1.5;
- the repository owns SQL but never starts processes or decides restart policy;
- the process layer owns bytes and correlation but never updates PostgreSQL;
- the supervisor owns state/lifecycle but never logs or stores resolved secrets;
- the sample adapter imports public contract-shaped types, not Core repositories or
  health evaluation; and
- conformance tests treat the executable as a black box over stdio.

## Deterministic sample adapter

The sample manifest uses `org.ubnetdef.espial.sample`, protocol `1.0`, read-only
`collect`, resource type `host`, and check type `sample.availability`.

Its validated non-secret configuration supports:

- scenario `healthy`, `warning`, or `critical`;
- deterministic fixture count within a small bound;
- response delay within a bounded test range; and
- test-only fault mode `none`, `malformed`, `oversized`, `crash_before_response`,
  `duplicate_response`, `stderr_flood`, or `refuse_shutdown`.

Fixture IDs, timestamps, measurements, and summaries are deterministic from the
request's `sent_at` test clock. The adapter never reports `stale`; Core derives
freshness. Fault switches are clearly sample/test behavior and are not generalized
into production adapter configuration.

## Test matrix

### Pure protocol and policy tests

- envelope kind/operation/payload/error matrix and strict unknown-field rejection;
- semantic versions including exact common minor, additive advertised minor,
  unknown major, malformed, duplicate, and no-common-version cases;
- manifest identity, capabilities, unique values, config schema, and secret-field
  validation;
- exact/beyond line, diagnostic ring, timeout, and request-count boundaries;
- typed errors contain no payload, config, filesystem, SQL, or canary-secret text;
- backoff sequence, cap, jitter bounds, persisted delay, healthy reset, manual stop,
  and cancellation using fake clock/timers; and
- runtime state transitions reject invalid or stale process generations.

### Black-box process and PostgreSQL integration tests

- ready → manifest → validate_config → health → collect → shutdown happy path;
- healthy, warning, and critical sample collection payloads validate against the v1
  resource/observation contracts;
- malformed/oversized stdout, partial EOF, early exit, slow response, wrong request
  ID/operation, terminal error, duplicate response, unsolicited output, and stderr
  flood;
- startup timeout, crash/restart/recovery, refusal to shut down, signal escalation,
  child reaping, and repeated start/stop without goroutine or pipe growth;
- instance state/version/error/backoff fields survive PostgreSQL reload;
- two integrations cannot update one another's instance or use an unregistered
  executable; and
- fake-resolver canary values are absent from command arguments, environment,
  diagnostics, errors, database rows, and captured logs.

Concurrency-bearing packages run under `go test -race`. Process tests use built
temporary binaries and short injected deadlines; they do not depend on shell
behavior, external network access, or production inventory.

## Implementation order

1. Add and integration-test migration `000005_adapter_runtime.sql`.
2. Implement strict domain envelope/manifest types, semantic validation, and
   version negotiation with schema-derived fixtures.
3. Implement the bounded NDJSON codec and safe error categories.
4. Implement the trusted registry, secret resolver interface, and redactor tests.
5. Implement one-generation process lifecycle, request correlation, and exhaustive
   cancellation/reaping tests.
6. Implement adapter-instance repository and supervisor state/backoff policy.
7. Build the standalone deterministic sample adapter.
8. Build the reusable black-box conformance harness and fault-mode tests.
9. Wire only the sample descriptor into Core configuration/startup without adding
   scheduling or ingestion.
10. Run formatting, vet, unit, race, PostgreSQL, contract, documentation, and full
    repository checks; update progress only after every gate passes.

## Questions and escalation points

No project-owner answer blocks the deterministic sample implementation. Ask before:

- allowing an API or database record to choose executable paths or arguments;
- adding a real secret-store resolver or placing secrets in child environment;
- accepting a protocol minor without an exact mutually advertised version;
- allowing concurrent requests per integration;
- changing restart/deadline/output limits based on assumed production behavior; or
- enabling adapter `events`, `notifications`, or `actions` before their consumer and
  authorization paths exist.

Production executable installation, OS sandboxing beyond process separation,
network egress, real credentials, and adapter-specific scale remain explicit later
deployment/integration inputs.

## Implementation evidence

Completed in this slice:

- migration `000005_adapter_runtime.sql` persists negotiated versions, stop/error
  times, failure count, and the next restart deadline with database constraints;
- `core/internal/adapters` provides strict envelope/manifest decoding, exact version
  negotiation, a hard-bounded NDJSON codec, immutable registry, secret resolver and
  redactor, bounded diagnostics, correlated process runtime, PostgreSQL instance
  repository, and restart supervisor;
- `core/cmd/espial-sample-adapter` and `core/internal/sampleadapter` provide the
  standalone deterministic healthy/warning/critical adapter and deliberate
  malformed, oversized, partial, slow, crash, duplicate, flood, error, mismatch,
  and shutdown-refusal modes;
- `core/internal/adapters/conformance` runs manifest, configuration, health,
  collection, normalized-batch, and shutdown checks against any trusted executable;
- the Core container installs both binaries, and typed configuration accepts only
  an absolute sample path while safe summaries expose only whether it is configured;
  and
- race and PostgreSQL tests cover exact limits, protocol/version rejection,
  one-request enforcement, timeouts, recovery, repeated reaping, shutdown
  escalation, persistent backoff, healthy reset, integration ownership, and a
  canary secret absent from arguments/environment and redacted from both stderr and
  protocol log diagnostics.

Slice 1.5 now consumes the tested `Session.Collect` boundary: it enumerates enabled
integrations, schedules calls, passes validated normalized batches to Slice 1.3,
and runs freshness deadlines. See the
[Slice 1.5 implementation record](SLICE_1_5_SCHEDULING_PIPELINE.md).
