#!/bin/sh
set -eu

if [ -z "${POSTGRES_REPLICATION_PASSWORD:-}" ]; then
  echo "POSTGRES_REPLICATION_PASSWORD is required" >&2
  exit 1
fi

psql -v ON_ERROR_STOP=1 \
  --username "$POSTGRES_USER" \
  --dbname "$POSTGRES_DB" \
  --set=replication_password="$POSTGRES_REPLICATION_PASSWORD" <<'SQL'
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tml_replicator') THEN
    CREATE ROLE tml_replicator WITH REPLICATION LOGIN;
  END IF;
END
$$;
ALTER ROLE tml_replicator PASSWORD :'replication_password';
SELECT pg_create_physical_replication_slot('tml_replica')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_replication_slots WHERE slot_name = 'tml_replica'
);
SQL

echo "host replication tml_replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"
