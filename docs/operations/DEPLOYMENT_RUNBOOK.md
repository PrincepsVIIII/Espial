# Deployment and upgrade runbook

## Local initialization

`npm run init` creates a missing `.env`, generates a random local PostgreSQL
password and mode-`0600` DSN secret, starts PostgreSQL, runs migrations, prompts
for the first administrator password without echoing it, and starts Core and Web.
It never installs a default administrator password. Existing environments are not
overwritten.

For an already initialized clone, use `npm run dev`; use the
[local-auth runbook](LOCAL_AUTH.md) for account commands.

## Production preflight

The release operator must record:

- approved host/failure domain, DNS name, inbound firewall rules, and an external
  heartbeat;
- immutable Core and Web image digests plus clean/triaged dependency and image scans;
- owner/app database credentials from an approved secret store;
- tested backup target, encryption, retention, restore target, and restore owner;
- adapter-specific egress allowlists and secret ownership;
- Mattermost exact host, every expected resolved CIDR, HTTPS port, certificate
  trust, webhook owner, and mounted token-file path;
- website-monitor exact hosts, every possible resolved IPv4/IPv6 CIDR, allowed
  ports, redirect ceiling, response limits, timeout policy, and protected-header
  secret-file ownership;
- memory/disk headroom against the measured Phase 1 baseline;
- a named rollback decision-maker and maintenance window.

Render `deployments/production/compose.yml` with `docker compose config`. Confirm
only the proxy publishes ports, `private` is internal, Core has
`ESPIAL_DATABASE_MIGRATE_ON_START=false`, and no rendered value contains a secret.

## Mattermost destination rotation and failure

1. Put only the new incoming-webhook token in a mode-`0600` secret file. Replace the
   Compose secret source and restart Core; do not paste the token into the API.
2. Confirm the destination host, every current DNS answer, and port remain in
   `ESPIAL_NOTIFICATION_APPROVED_HOSTS`, `ESPIAL_NOTIFICATION_APPROVED_CIDRS`, and
   `ESPIAL_NOTIFICATION_ALLOWED_PORTS`. Treat an unexpected extra DNS answer as a
   policy failure, not a reason to widen the CIDR casually.
3. In `/alerts/notifications`, replace the destination using the opaque secret
   filename, then send the explicitly labeled test. Follow its receipt to audit and
   confirm delivery evidence reaches `Delivered`.
4. For `Waiting to retry`, inspect the safe reason and provider health. Espial stops
   after six attempts. For `Failed`, correct configuration/provider rejection and
   send a new test. For `Dead letter`, preserve the row and investigate; automatic
   replay is intentionally unavailable.
5. Rotate/revoke the old Mattermost webhook after the new test succeeds. Capture no
   webhook URL/token, provider body, or secret-file contents in tickets or logs.

During shutdown, Core cancels new claims and allows active HTTP calls to return under
their configured timeout. Expired leases are reclaimed after restart. Do not edit
intent/attempt tables or reset attempts to force replay.

## Website monitor allowlists and diagnosis

1. Set `ESPIAL_WEBCHECK_APPROVED_HOSTS` to exact lower-case destination names,
   `ESPIAL_WEBCHECK_APPROVED_CIDRS` to the narrow networks containing every expected
   DNS answer, and `ESPIAL_WEBCHECK_ALLOWED_PORTS` to the required ports. An empty
   host or CIDR list fails closed. Do not add a broad private network merely to make
   a check pass.
2. Put each protected header value in its own mode-`0600` file beneath
   `ESPIAL_WEBCHECK_SECRET_DIRECTORY`. Configure only its opaque file name through
   `/webpages/monitors`; never put the value in the URL, API JSON, or a ticket.
3. Follow the mutation receipt to Audit, request a manual check, then inspect the
   Webpages detail stages and safe reason code. DNS policy rejection, TCP refusal,
   TLS verification failure, response timeout, status mismatch, body/header limit,
   and content mismatch remain distinct without retaining response content.
4. When DNS changes, approve every new answer deliberately before replacing the
   monitor configuration or deployment allowlist. Redirect targets require their
   own host/address/port approval; protected headers cannot cross origins.
5. Adapter failure remains isolated. Use integration runtime evidence to diagnose
   the process; normal freshness will make the webpage stale and then unknown. Do
   not edit observation, signal, or incident rows to mask the failure.

## Upgrade or rollback

1. Read release notes and migration files. Take and verify a backup using
   [BACKUP_RESTORE.md](BACKUP_RESTORE.md).
2. Pull images by digest and scan them. Do not deploy an unowned critical/high
   finding.
3. Stop Web/Core while keeping PostgreSQL available. Run the new `migrate` image
   once with the owner DSN. A failed migration stops the release.
4. Start Core and wait for readiness; start Web and proxy; exercise login, a Viewer
   denial, dashboard reads, adapter collection, and SSE reconnect.
5. Observe error logs, adapter restarts, database saturation, and memory for the
   maintenance window.

Application images can be rolled back independently when their database contract
is compatible. Migrations are forward-only; never edit the schema manually or
assume an old binary can use a newer schema. When compatibility is not guaranteed,
restore the pre-upgrade backup into a clean database and deploy the matching image
set.

## Complete shutdown

Run Compose `stop` and allow the configured grace period. Core cancels scheduling,
terminates supervised adapters, persists stopped state, and closes PostgreSQL.
The final Phase 1 exercise measured Core shutdown at 647 ms during collection. After all
containers stop, verify there is no remaining Espial adapter process. `down` removes
containers and networks but retains named data volumes. Never add `--volumes` to a
routine shutdown.
