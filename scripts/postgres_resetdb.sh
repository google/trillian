#!/bin/bash

set -e

usage() {
  cat <<EOF
$(basename $0) [--force] [--verbose] ...
All unrecognised arguments will be passed through to the 'postgres' command.
Accepts environment variables:
- PG_ROOT_USER: A user with sufficient rights to create/reset the Trillian
  database (default: `postgres`).
- PG_HOST: The hostname of the PG server (default: localhost).
- PG_PORT: The port the PG server is listening on (default: 5432).
- PG_DATABASE: The name to give to the new Trillian user and database
  (default: test).
EOF
}

die() {
  echo "$*" > /dev/stderr
  exit 1
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${PG_ROOT_USER+x} ] && PG_ROOT_USER="postgres"
  [ -z ${PG_HOST+x} ] && PG_HOST="localhost"
  [ -z ${PG_PORT+x} ] && PG_PORT="5432"
  [ -z ${PG_DATABASE+x} ] && PG_DATABASE="test"
  FLAGS=()

  FLAGS+=(-U "${PG_ROOT_USER}")
  FLAGS+=(--host "${PG_HOST}")
  FLAGS+=(--port "${PG_PORT}")
}

main() {
  collect_vars "$@"
  
  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  echo "Warning: about to destroy and reset database '${PG_DATABASE}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      psql "${FLAGS[@]}" -c "DROP DATABASE IF EXISTS ${PG_DATABASE};" || \
        die "Error: Failed to drop database '${PG_DATABASE}'."
      psql "${FLAGS[@]}" -c "CREATE DATABASE ${PG_DATABASE};" || \
        die "Error: Failed to create database '${PG_DATABASE}'."
      psql "${FLAGS[@]}" -d ${PG_DATABASE} -f ${TRILLIAN_PATH}/storage/postgres/storage.sql || \
        die "Error: Failed to create tables in '${PG_DATABASE}' database."
      echo "Reset Complete"
  fi
}

main "$@"
