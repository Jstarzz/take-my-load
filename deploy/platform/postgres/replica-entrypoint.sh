#!/bin/sh
set -eu

: "${PGDATA:=/var/lib/postgresql/data}"
: "${POSTGRES_REPLICATION_PASSWORD:?POSTGRES_REPLICATION_PASSWORD is required}"

if [ ! -s "$PGDATA/PG_VERSION" ]; then
  echo "waiting for postgres-primary..."
  until pg_isready -h postgres-primary -p 5432 >/dev/null 2>&1; do
    sleep 1
  done

  rm -rf "$PGDATA"/*
  chown -R postgres:postgres "$PGDATA"
  export PGPASSWORD="$POSTGRES_REPLICATION_PASSWORD"
  gosu postgres pg_basebackup \
    -h postgres-primary \
    -p 5432 \
    -U tml_replicator \
    -D "$PGDATA" \
    -Fp \
    -Xs \
    -P \
    -R \
    -S tml_replica
fi

exec docker-entrypoint.sh postgres -c hot_standby=on
