#!/bin/bash

set -e

usage() {
  cat <<EOF
$(basename $0) [--force] [--verbose] ...
All unrecognised arguments will be passed through to the 'psql' command.
Accepts environment variables:
- POSTGRES_ROOT_USER: A user with sufficient rights to create/reset the Trillian
  database (default: `postgres`).
- POSTGRES_HOST: The hostname of the PG server (default: localhost).
- POSTGRES_PORT: The port the PG server is listening on (default: 5432).
- POSTGRES_DB: The name to give to the new Trillian database (default: test).
- POSTGRES_USER: The name to give to the new Trillian user (default: test).
- POSTGRES_PASSWORD: The password to use for the new Trillian user
  (default: zaphod). Note: the password cannot contain single quotes.
EOF
}

die() {
  echo "$*" > /dev/stderr
  exit 1
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${POSTGRES_ROOT_USER+x} ] && POSTGRES_ROOT_USER="postgres"
  [ -z ${POSTGRES_HOST+x} ] && POSTGRES_HOST="localhost"
  [ -z ${POSTGRES_PORT+x} ] && POSTGRES_PORT="5432"
  [ -z ${POSTGRES_DB+x} ] && POSTGRES_DB="test"
  [ -z ${POSTGRES_USER+x} ] && POSTGRES_USER="test"
  [ -z ${POSTGRES_PASSWORD+x} ] && POSTGRES_PASSWORD="zaphod"
  FLAGS=()

  FLAGS+=(-U "${POSTGRES_ROOT_USER}")
  FLAGS+=(--host "${POSTGRES_HOST}")
  FLAGS+=(--port "${POSTGRES_PORT}")

  # handle flags
  FORCE=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force) FORCE=true ;;
      --help) usage; exit ;;
      *) FLAGS+=("$1")
    esac
    shift 1
  done
}

main() {
  collect_vars "$@"

  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  echo "Warning: about to destroy and reset database '${POSTGRES_DB}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      psql "${FLAGS[@]}" -c "DROP DATABASE IF EXISTS ${POSTGRES_DB};" || \
        die "Error: Failed to drop database '${POSTGRES_DB}'."
      psql "${FLAGS[@]}" -c "CREATE DATABASE ${POSTGRES_DB};" || \
        die "Error: Failed to create database '${POSTGRES_DB}'."
      psql "${FLAGS[@]}" -d ${POSTGRES_DB} -f ${TRILLIAN_PATH}/storage/postgres/schema/storage.sql || \
        die "Error: Failed to create tables in '${POSTGRES_DB}' database."
      if ! psql "${FLAGS[@]}" -t -c "SELECT 1 FROM pg_user WHERE usename = '${POSTGRES_USER}'" | grep -q 1; then
        psql "${FLAGS[@]}" -c "CREATE USER ${POSTGRES_USER} WITH PASSWORD '${POSTGRES_PASSWORD}' CREATEDB;" || \
          die "Error: Failed to create user '${POSTGRES_USER}'."
      fi
      psql "${FLAGS[@]}" -d ${POSTGRES_DB} -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO ${POSTGRES_USER};" || \
        die "Error: Failed to grant '${POSTGRES_USER}' user all privileges on tables in '${POSTGRES_DB}'."
      psql "${FLAGS[@]}" -d ${POSTGRES_DB} -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO ${POSTGRES_USER};" || \
        die "Error: Failed to grant '${POSTGRES_USER}' user all privileges on sequences in '${POSTGRES_DB}'."
      echo "Reset Complete"
  fi
}

main "$@"
