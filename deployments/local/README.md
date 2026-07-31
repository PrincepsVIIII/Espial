# Local development stack

The local stack runs PostgreSQL 17, Espial Core, and Espial Web as separate containers. It is for
development and tests only; its example password is intentionally public and must
never be reused.

## Start

For a new clone, install dependencies and run the initializer:

```sh
npm install
npm --prefix web ci
npm run init
```

It creates a random local database secret and `.env` when absent, runs migrations,
prompts for the initial administrator without echo, and starts the stack. It never
creates a default password. Use `npm run dev` on later starts.

The default host ports are Web `5173`, Core `18080`, and PostgreSQL `55432`, all
bound to loopback. Change `ESPIAL_WEB_PORT`, `ESPIAL_CORE_PORT`, or `POSTGRES_PORT`
in `.env` if needed.

If the database exists but has not been bootstrapped, create the first local
administrator in a second terminal:

```sh
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin bootstrap --username admin
```

Then open `http://localhost:5173/login`. See the
[local authentication runbook](../../docs/operations/LOCAL_AUTH.md) for security
controls and transition cautions.

Check the service:

```sh
curl http://127.0.0.1:18080/api/v1/health/live
curl http://127.0.0.1:18080/api/v1/health/ready
```

## Stop and data

`npm run down` stops and removes containers while retaining the named PostgreSQL
volume. To remove that development data deliberately, run the Compose `down
--volumes` form yourself after confirming the local project name; the Makefile does
not delete data.

The Core image is a static, non-root runtime image. PostgreSQL receives its local
development DSN through a mounted Compose secret file rather than an environment
variable. The committed value is not suitable for any shared or production host.
