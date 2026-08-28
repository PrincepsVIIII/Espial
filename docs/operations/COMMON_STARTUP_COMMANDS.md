# Common startup commands

Run these commands from the repository root. They use the local Docker Compose
stack: PostgreSQL, Espial Core, and Espial Web.

## First startup

Install the root and web dependencies, create local configuration, bootstrap the
database, and start the stack:

```sh
npm install
npm --prefix web ci
npm run init
```

The initializer prompts for the initial local administrator. After it completes,
open <http://localhost:5173>.

## Start or rebuild

Rebuild the local images and run the stack in the foreground:

```sh
npm run dev
```

Use `Ctrl+C` to stop following the foreground process. Then run the stop command
below to remove the containers cleanly if needed.

To rebuild and run the stack in the background instead:

```sh
docker compose --env-file .env -f deployments/local/compose.yml up --build -d
```

## Stop

Stop and remove the local containers while keeping the PostgreSQL development
volume:

```sh
npm run down
```

## Restart

For a clean local restart that retains development data:

```sh
npm run down
npm run dev
```

To restart in the background, replace the second command with:

```sh
docker compose --env-file .env -f deployments/local/compose.yml up --build -d
```

## Check status and health

```sh
docker compose --env-file .env -f deployments/local/compose.yml ps
curl http://127.0.0.1:18080/api/v1/health/live
curl http://127.0.0.1:18080/api/v1/health/ready
```

The default host ports are Web `5173`, Core `18080`, and PostgreSQL `55432`.
Configuration and data-retention details are in the
[local development stack guide](../../deployments/local/README.md).
