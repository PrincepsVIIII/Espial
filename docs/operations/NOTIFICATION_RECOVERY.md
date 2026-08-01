# Notification queue and Mattermost recovery

Espial guarantees one database intent per incident event and destination. Remote
delivery is at least once: a process loss after Mattermost accepts a request can
repeat the HTTP request. The stable Espial event ID in the message is the duplicate
recognition key.

## Diagnose

1. Inspect `/alerts/notifications` and the incident delivery section. Distinguish
   Queued, Attempting, Waiting to retry, Failed, Dead letter, and Suppressed.
2. Read private Core `/metrics`. Check `espial_notification_intents` by bounded
   state and `espial_notification_oldest_due_age_seconds`; never scrape the public
   proxy because the production Caddy route intentionally excludes `/metrics`.
3. Confirm the exact Mattermost hostname, every current DNS answer, allowed port,
   TLS trust, and mounted secret reference. Do not log or paste the token.
4. A `429`, timeout, connection failure, or `5xx` retries with bounded backoff. A
   provider rejection, redirect, policy rejection, or invalid payload is terminal.

## Recover

- Restore the provider or correct the destination configuration. Pending retry and
  expired Attempting leases resume after Core restart without creating a second
  intent.
- Rotate a token by mounting the replacement file, replacing the destination with
  its opaque reference, and sending a labeled test. Revoke the old token only after
  the test is Delivered.
- Dead-letter and Failed intents are immutable evidence. Phase 2 intentionally has
  no arbitrary replay endpoint because replay can resend stale incident messages.
  Preserve the row, fix the cause, and use a labeled destination test. A later real
  incident event creates its own intent under normal policy.
- Suppressed intents are terminal and must not be replayed after silence expiry.

Never update intent state, attempts, lease timestamps, or secret references with
SQL. Escalate a growing oldest-due age before increasing concurrency; first rule out
DNS, TLS, database saturation, provider throttling, and a retry storm.

## Queue thresholds

The Slice 2.7 reference profile drains 250 deliveries plus 1,000 signals in under
30 seconds and a 10× oversized profile in under two minutes on the recorded
development runner. Treat notification oldest-due age above 30 seconds as an
investigation threshold and above two minutes as backlog breach until site-specific
evidence establishes a stricter objective.
