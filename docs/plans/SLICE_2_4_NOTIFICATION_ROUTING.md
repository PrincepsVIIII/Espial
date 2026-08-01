# Slice 2.4 delivery record: notification routing and Mattermost

Slice 2.4 implements the notification boundary defined by the Phase 2 plan. Core now
commits destination-independent intents atomically with detected, severity-change,
recurrence, and recovery timeline events. Duplicate event/destination writes are
idempotent. A matching incident, rule, or resource silence becomes terminal
`suppressed` evidence and never queues a surprise delivery.

The PostgreSQL outbox stores destination metadata, intents, bounded retry state, and
append-only attempts. A two-worker default claim loop uses leases and `SKIP LOCKED`,
reclaims restart-expired work, caps delivery at six attempts, dead-letters exhausted
work, shuts down through process cancellation, publishes REST-refetch invalidations,
and exposes low-cardinality queue/attempt metrics through the notification service.

Mattermost delivery remains outside incident policy. The driver sends safe fixed
markdown with stable delivery IDs, explicit test labeling, escaped source/operator
text, and neutralized mentions. Incident links derive only from `ESPIAL_PUBLIC_URL`.
HTTPS certificate verification, exact host plus resolved-CIDR plus port allowlists,
address-pinned dialing, redirect/proxy rejection, timeouts, and response caps are
mandatory. Tokens resolve from opaque filenames beneath the mounted secret directory;
destination reads, audit summaries, API failures, and startup summaries omit protected
values.

Administrator APIs and `/alerts/notifications` provide redacted list/detail,
create/full-replace, explicitly labeled test, and delivery history operations. Every
mutation requires origin, CSRF, idempotency, and (where replacing/testing) version
preconditions, then returns a correlation receipt linked to audit. Incident details
show viewer-readable delivery status, time, attempts, and safe reason alongside
immutable notification timeline evidence.

Verification includes PostgreSQL duplicate/silence/append-only/retry/dead-letter/
restart cases; race execution; Mattermost success, timeout, 429, 4xx, 5xx, response
cap, redirect, escaping, and SSRF/rebinding matrices; API permission and precondition
checks; contract fixtures; and responsive/accessibility browser checks at 1440px,
1280px, and narrow viewports.

Operational constraints remain deliberate: empty allowlists disable destination
configuration, test sends are real asynchronous deliveries, webhook rotation requires
a secret-file replacement and Core restart, and dead-letter replay is a manual future
capability rather than an implicit resend button.
