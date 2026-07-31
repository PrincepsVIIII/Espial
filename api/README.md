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

Schemas use JSON Schema draft 2020-12. A major directory version represents an
incompatible contract boundary; additive compatible changes may update v1 after
fixtures and consumers pass CI. `$id` values are stable identifiers, not a promise
that the referenced host currently serves the files.

Positive and negative examples live in [`fixtures/v1`](fixtures/v1). Run
`npm run contracts` from the repository root to validate all schemas and fixtures.
Run `npm run generate` to regenerate
[`web/src/lib/api/generated.ts`](../web/src/lib/api/generated.ts).

See [API conventions](../docs/architecture/API_CONVENTIONS.md) and the
[adapter protocol](../docs/architecture/ADAPTER_PROTOCOL.md).
