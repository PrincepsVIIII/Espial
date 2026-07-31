# ADR-0003: NDJSON over supervised stdio for the first adapter transport

- Status: Accepted
- Date: 2026-07-30

## Context

The adapter boundary must be language-neutral, easy for students to implement, and
isolated from Core failures. The backend plan permits supervised stdio or local
HTTP, but Phase 1 needs one concrete transport.

## Decision

Use newline-delimited JSON request/response envelopes over stdin/stdout for locally
supervised Phase 1 adapters. Each request has a unique ID and exactly one terminal
response. Notifications such as adapter log or event messages omit a request ID.
Stderr is diagnostic output only. Protocol schemas are versioned under `api/`.

## Consequences

Core owns process lifecycle, deadlines, bounded message sizes, and restart policy.
Adapters must never write non-protocol output to stdout. Local HTTP may be added
later as a second transport behind the same logical contract, not as a replacement
for normalized models.
