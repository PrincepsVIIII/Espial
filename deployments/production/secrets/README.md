# Production secret files

Create five mode-`0600` files outside version control before rendering the stack:

- `postgres_owner_password`: random PostgreSQL migration-owner password;
- `postgres_app_password`: separate random runtime password;
- `database_owner_dsn`: `postgres://espial_owner:<encoded-password>@postgres:5432/espial?sslmode=disable`;
- `database_app_dsn`: `postgres://espial_app:<encoded-password>@postgres:5432/espial?sslmode=disable`.
- `mattermost_operations_webhook`: only the incoming-webhook token, without a URL,
  newline, or surrounding quotes. Its destination uses the secret reference
  `mattermost_operations_webhook`.

Do not place real values in this directory in a clone. Prefer the host's secret
manager and set the five file paths in the production `.env` file. Rotate the
Mattermost token by replacing the mounted file and restarting Core; never put the
token in an environment variable, destination JSON, logs, or screenshots.
