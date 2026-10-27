#!/bin/sh
set -eu
cd "$(dirname "$0")"

if [ ! -f root-password ]; then
    (umask 077; openssl rand -hex 16 > root-password)
fi

exec docker compose up -d --wait --wait-timeout 120
