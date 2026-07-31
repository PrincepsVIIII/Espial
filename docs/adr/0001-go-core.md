# ADR-0001: Go for Espial Core

- Status: Accepted
- Date: 2026-07-30

## Context

Espial Core coordinates long-running network I/O, schedules, adapter processes,
database transactions, live clients, cancellation, and graceful shutdown. It must
remain approachable to rotating student contributors and deploy as a small number
of artifacts.

## Decision

Implement Espial Core in Go. Begin with Go 1.26 and a standard-library-compatible
HTTP stack. Keep application packages under `core/internal` and keep public adapter
contracts language-neutral under `api/`.

## Consequences

Concurrency ownership, timeouts, cancellation, race testing, and goroutine cleanup
are mandatory review concerns. Dependencies must justify themselves at package
boundaries; the project will not adopt a broad application framework by default.
