# PostgreSQL backup and restore runbook

Backups are useful only after an isolated restore proves authentication, current
health, audit history, and schema state. The production backup destination,
encryption mechanism, schedule, retention, and operator remain deployment blockers
until named in the [environment inventory](ENVIRONMENT_INVENTORY.md).

## Logical backup

Run `pg_dump` from the pinned PostgreSQL image or database host using the migration
owner credential. Write a custom-format backup directly to the approved encrypted
backup target; do not place it in the repository or shell history. Record image
digest, UTC time, database version, migration version, checksum, and operator.

Example command shape (paths and secret injection are host-specific):

```sh
pg_dump --format=custom --no-owner --no-acl --dbname="$ESPIAL_BACKUP_DSN" --file=/approved/target/espial.dump
```

## Restore drill

1. Create an empty isolated PostgreSQL 17 database with fresh owner/app roles.
2. Verify the backup checksum, then restore with `pg_restore --exit-on-error
   --clean --if-exists --no-owner --no-acl` as the owner.
3. Run the matching migration image; it must report no unexpected drift.
4. Start matching Core/Web images against the isolated database.
5. Verify administrator login, Viewer `403`, integration configuration and state,
   current resources, and required audit events. Do not contact production targets.
6. Record duration and outcome, then securely dispose of the temporary copy.

`npm run acceptance:phase1` automates this shape with disposable databases and
non-production sample data. It is not a substitute for a scheduled restore of the
real encrypted production artifact. Run a restore drill before first production
use, after database-image changes, and at the documented operational cadence.
