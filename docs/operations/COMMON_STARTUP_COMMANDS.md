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
npm start
```

`npm run dev` remains available. If `make` is installed, use `make start`
instead.

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
npm stop
```

`npm run down` remains available. If `make` is installed, use `make stop`
instead.

## Restart

For a clean local restart that retains development data:

```sh
npm restart
```

If `make` is installed, use `make restart` instead. `restart` is equivalent to
stopping the local containers and starting the rebuilt stack; it retains the
PostgreSQL development volume.

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
