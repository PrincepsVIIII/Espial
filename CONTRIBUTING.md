# Contributing to Espial

Espial is split into independently testable components with versioned contracts.
Read the [documentation index](docs/README.md) and relevant accepted ADRs before
changing a shared boundary.

## Repository boundaries

- `core/`: Go service and database migrations
- `web/`: SvelteKit/TypeScript presentation and browser session client
- `adapters/`: official and reference out-of-process adapters
- `api/`: language-neutral schemas and representative fixtures
- `deployments/`: local and production deployment examples
- `docs/`: decisions, architecture, plans, design, and operations records

Do not place vendor-specific behavior in Core or duplicate Core authorization and
health evaluation in Web.

## Formatting and checks

Phase 1 will expose a root `make check` that runs the same non-networked checks as
CI. Until then, every change follows these minimum conventions:

- Go: `gofmt`, `go vet ./...`, `go test ./...`, and focused race tests for concurrent
  packages; errors include operational context and contexts cross I/O boundaries.
- Web: Prettier, ESLint, Svelte checks, TypeScript strict mode, component/unit tests,
  and accessibility checks.
- Schemas: parse every JSON file, validate schemas against draft 2020-12, and run
  positive and negative fixtures against every changed contract.
- Docs: preserve relative links, wrap readable prose where practical, and update
  indexes when a document is added.

Formatting-only commits should not be mixed with behavioral changes across an
entire component.

## Testing expectations

Tests live beside Go packages and frontend units. Cross-process and database tests
live in clearly named integration suites and start their own isolated resources.
Every bug fix adds a regression test. Time, random jitter, process launching, and
external I/O use controllable boundaries so tests remain deterministic.

Critical Phase 1 cases include authentication/role denial, session revocation,
malformed adapter output, output size limits, collection timeout, adapter crash and
restart, stale/unknown transitions, transaction rollback, SSE reconnect/resync, and
graceful shutdown. Run `go test -race` on concurrency-bearing packages in CI.

## CI convention

Pull requests run contract/docs checks, Core lint/unit/integration tests, Web
lint/type/unit/accessibility tests, and a vertical-slice test with ephemeral
PostgreSQL and the sample adapter. The default branch additionally builds immutable
artifacts. Dependency downloads are pinned by lockfiles/checksums; release jobs do
not run untrusted pull-request code with repository secrets.

## Security and change review

- Never commit credentials, tokens, raw production payloads, internal passwords,
  private keys, or `.env` files.
- Redact logs and fixtures while retaining useful error categories.
- Authentication, authorization, secret handling, migrations, adapter sandboxing,
  audit behavior, or public API changes require a second reviewer.
- Breaking a v1 contract requires an ADR and migration/deprecation plan.
- Controlled actions remain out of scope until their roadmap phase.

## Documentation changes

Use an ADR for durable cross-component choices, a plan for implementation order,
and `docs/operations` for environment facts. Unknown facts are written as
`TBD (owner)` with the milestone they block.
