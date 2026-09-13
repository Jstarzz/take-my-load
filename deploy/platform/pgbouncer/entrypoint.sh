#!/bin/sh
set -eu

: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"

umask 077
printf '"%s" "%s"\n' "$POSTGRES_USER" "$POSTGRES_PASSWORD" > /tmp/tml-userlist.txt
exec /opt/pgbouncer/pgbouncer /opt/tml/pgbouncer.ini
