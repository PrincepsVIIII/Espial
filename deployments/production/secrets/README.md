# Production secret files

Create four mode-`0600` files outside version control before rendering the stack:

- `postgres_owner_password`: random PostgreSQL migration-owner password;
- `postgres_app_password`: separate random runtime password;
- `database_owner_dsn`: `postgres://espial_owner:<encoded-password>@postgres:5432/espial?sslmode=disable`;
- `database_app_dsn`: `postgres://espial_app:<encoded-password>@postgres:5432/espial?sslmode=disable`.

Do not place real values in this directory in a clone. Prefer the host's secret
manager and set the four file paths in the production `.env` file.
