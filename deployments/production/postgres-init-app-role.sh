#!/bin/sh
set -eu

app_password="$(tr -d '\r\n' </run/secrets/postgres_app_password)"
if [ -z "$app_password" ]; then
  echo "postgres application password secret is empty" >&2
  exit 1
fi

psql --set=ON_ERROR_STOP=1 --set=app_password="$app_password" --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE espial_app LOGIN PASSWORD :'app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION;
GRANT CONNECT ON DATABASE espial TO espial_app;
GRANT USAGE ON SCHEMA public TO espial_app;
ALTER DEFAULT PRIVILEGES FOR ROLE espial_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO espial_app;
ALTER DEFAULT PRIVILEGES FOR ROLE espial_owner IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO espial_app;
SQL
