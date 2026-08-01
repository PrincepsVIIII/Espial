# ADR 0009: Website monitors are typed integration projections

- Status: Accepted
- Date: 2026-08-01

## Context

Website availability needs an administrator-facing monitor model, but Espial
already has one authoritative registry for adapter identity, enablement, schedule,
configuration, secrets, runtime state, and collection evidence. A separate website
configuration table would create conflicting schedulers and mutation paths.

Website checks also cross an SSRF-sensitive network boundary. DNS answers and
redirects can change after configuration, while protected request headers and
response content must not enter Core's normal evidence paths.

## Decision

A website monitor is a typed projection of one `integrations` row whose adapter ID
is `org.ubnetdef.espial.webcheck`. Its opaque monitor ID is the integration ID.
Only the website-monitor API creates or replaces these rows; the generic integration
mutation service rejects this adapter ID. Reads redact exact content values and
secret references while retaining safe configuration facts.

Core supervises a separate read-only `webcheck` process through the existing NDJSON
adapter protocol and scheduler. Manual checks coalesce through the same per-monitor
and global concurrency controls. The adapter emits one `webpage` resource and one
aggregate `website.availability` observation per attempt. Bounded measurements and
the completed-stage list preserve DNS, TCP, TLS, HTTP, response-size, redirect, and
latency evidence without creating several incidents for one endpoint outage.

Core passes the adapter only explicit policy environment variables; it does not
inherit ambient proxy settings. Each request and redirect hop validates the exact
host and port, resolves under a deadline, requires every answer to match an approved
CIDR, and dials only the approved resolved addresses. TLS uses the configured
hostname and approved roots. Cross-origin redirects never receive protected
headers. URL user information and secret-like query keys are rejected.

## Consequences

- The existing integration registry remains the single configuration and scheduling
  authority; no website-monitor table exists.
- Configuration changes restart the supervised generation, and manual requests do
  not bypass scheduler bounds.
- The API, observations, audit summaries, logs, and live invalidations contain no
  secret header value or response body. Content-match configuration is exposed only
  as a boolean.
- A website-specific incident rule wins by resource-kind/check-type specificity, so
  a failed aggregate observation enters the same durable incident engine as other
  resources and produces one active fingerprint.
- Empty host or CIDR allowlists fail closed. Operators must approve both the name and
  every possible address before a monitor can collect.
