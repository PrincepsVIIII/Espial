# Incident, rule, maintenance, and silence operations

Use Core-authorized routes and preserve every receipt. Direct database changes are
not an operational control and destroy the evidence needed to explain an incident.

## Incident workflow

1. Open the incident from `/alerts` or its direct `/alerts/{id}` URL and verify the
   resource, check, current severity, source time, and latest timeline evidence.
2. Acknowledge only when an Operator has accepted responsibility. Assign an enabled
   Operator or Administrator, move to Investigating, and add bounded plain-text
   notes containing facts rather than credentials or copied response bodies.
3. If Core returns `412`, stop. Refetch the incident, review the intervening event,
   and deliberately repeat the action with the new version. Never bypass the ETag.
4. Resolve only after Core reports Recovered. The resolution note is required and
   becomes immutable evidence. A recurring condition before resolution reopens the
   same incident; a later condition after resolution creates a new episode.
5. Record the request ID from every mutation. Administrators follow the supplied
   `/audit?correlation_id=...` link and verify one matching redacted audit event.

Viewer accounts may read incidents and delivery evidence but must receive `403` for
operator actions. Do not temporarily elevate a Viewer merely to acknowledge an
incident; assign the work to an authorized Operator.

## Rule rollout and rollback

1. Read the current rule and retain its redacted definition and ETag in the change
   record. Use Preview with representative integration, resource, check, state, and
   controlled reason-code inputs. Resolve unexpected overlaps before enabling it.
2. Start with the narrowest correct resource/check scope. Use bounded debounce and
   recovery occurrences. Enabling a broad rule without preview is a release error.
3. Replace the rule through `/alerts/rules`, preserve the receipt, and follow it to
   Audit. Watch signal queue age, incident creation, and notification queue state.
4. Roll back by fetching the current rule, then replacing it with the recorded
   prior definition using the current ETag and a new idempotency key. Disabling is
   preferable to deleting evidence; Phase 2 does not delete rules.
5. A rule change does not erase existing incidents. Operate each incident according
   to its authoritative state and document why policy changed.

## Maintenance windows

Maintenance is appropriate for expected work that should not open or worsen a
matching incident. Configure an explicit UTC start/end and the smallest integration,
resource, or check scope. Confirm the UI continues to show raw failure evidence and
labels only effective state as Maintenance. At expiry, Core evaluates the current
health immediately; a persistent failure begins normal debounce from expiry.

Revoke an incorrect window through `/alerts/suppressions`, retain the receipt, and
verify Audit. Never edit health, signal, or incident rows to simulate maintenance.

## Notification silences

A silence suppresses delivery only. It cannot hide an incident or change health.
Use the narrowest incident, rule, or resource target, a factual reason, and an
explicit expiry. Matching intents become terminal Suppressed evidence and are not
queued for surprise delivery later. Revoke through the UI and verify its receipt;
previously suppressed intents remain preserved.

## Dry-run record

Before production release, an Operator who did not author this runbook must execute
the warning-to-resolution workflow, one stale ETag conflict, a rule preview and
rollback, one maintenance expiry, and one silence. Record operator, UTC date,
environment, release digest, receipts, deviations, and outcome in the deployment
change record. This repository cannot self-attest that external review.
