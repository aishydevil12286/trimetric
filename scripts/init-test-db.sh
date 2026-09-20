#!/bin/sh
# Creates the database the Go tests use, alongside the application database.
# Postgres runs everything in /docker-entrypoint-initdb.d once, on first start
# of an empty data directory.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<SQL
  CREATE DATABASE test_trimetric OWNER $POSTGRES_USER;
SQL
