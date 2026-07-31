# Espial shared API contracts

This directory contains language-neutral, versioned contracts shared by Core,
adapters, and Web. Planning prose belongs in `docs/`; executable schemas belong
here.

## Version 1 schemas

- [`resource.schema.json`](schemas/v1/resource.schema.json): normalized discovered resource
- [`observation.schema.json`](schemas/v1/observation.schema.json): timestamped health/check result
- [`event.schema.json`](schemas/v1/event.schema.json): normalized domain/live event envelope
- [`adapter-manifest.schema.json`](schemas/v1/adapter-manifest.schema.json): adapter identity and capabilities
- [`adapter-envelope.schema.json`](schemas/v1/adapter-envelope.schema.json): NDJSON process message envelope
- [`api-error.schema.json`](schemas/v1/api-error.schema.json): safe v1 error envelope
- [`overview.schema.json`](schemas/v1/overview.schema.json): Dashboard monitoring summary
- [`resource-view.schema.json`](schemas/v1/resource-view.schema.json): resource detail and current health
- [`integration-view.schema.json`](schemas/v1/integration-view.schema.json): integration, runtime, and collection state
- [`audit-event.schema.json`](schemas/v1/audit-event.schema.json): redacted administrative audit record
- [`live-invalidation.schema.json`](schemas/v1/live-invalidation.schema.json): SSE invalidation payload
- [`integration-write.schema.json`](schemas/v1/integration-write.schema.json): integration creation and configuration replacement

Schemas use JSON Schema draft 2020-12. A major directory version represents an
incompatible contract boundary; additive compatible changes may update v1 after
fixtures and consumers pass CI. `$id` values are stable identifiers, not a promise
that the referenced host currently serves the files.

Positive and negative examples live in [`fixtures/v1`](fixtures/v1). Run
`npm run contracts` from the repository root to validate all schemas and fixtures.
That command also checks [`openapi/v1.json`](openapi/v1.json), every operation ID,
all local external schema references, and shared-schema reuse.
Run `npm run generate` to regenerate
[`web/src/lib/api/generated.ts`](../web/src/lib/api/generated.ts).

See [API conventions](../docs/architecture/API_CONVENTIONS.md) and the
[adapter protocol](../docs/architecture/ADAPTER_PROTOCOL.md).
