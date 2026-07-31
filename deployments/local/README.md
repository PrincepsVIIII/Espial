# Local development stack

The local stack runs PostgreSQL 17 and Espial Core as separate containers. It is for
development and tests only; its example password is intentionally public and must
never be reused.

## Start

From the repository root:

```sh
cp .env.example .env
npm install
npm run dev
```

The default host ports are Core `18080` and PostgreSQL `55432`, both bound to
loopback. Change `ESPIAL_CORE_PORT` or `POSTGRES_PORT` in `.env` if needed.

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
