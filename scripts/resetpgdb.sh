#!/bin/bash

set -e

usage() {
  cat <<EOF
$(basename $0) [--force] [--verbose] ...
All unrecognised arguments will be passed through to the 'psql' command.
Accepts environment variables:
- POSTGRESQL_ROOT_USER: A user with sufficient rights to create/reset the Trillian
  database (default: postgres).
- POSTGRESQL_ROOT_PASSWORD: The password for \$POSTGRESQL_ROOT_USER (default: none).
- POSTGRESQL_HOST: The hostname of the PostgreSQL server (default: localhost).
- POSTGRESQL_PORT: The port the PostgreSQL server is listening on (default: 5432).
- POSTGRESQL_DATABASE: The name to give to the new Trillian user and database
  (default: defaultdb).
- POSTGRESQL_USER: The name to give to the new Trillian user (default: test).
- POSTGRESQL_PASSWORD: The password to use for the new Trillian user
  (default: zaphod).
- POSTGRESQL_IN_CONTAINER: If set, the script will assume it is running in a Docker
  container and will exec into the container to operate (default: false).
- POSTGRESQL_CONTAINER_NAME: The name of the Docker container to exec into (default:
  pgsql).
EOF
}

die() {
  echo "$*" > /dev/stderr
  exit 1
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${POSTGRESQL_ROOT_USER+x} ] && POSTGRESQL_ROOT_USER="postgres"
  [ -z ${POSTGRESQL_HOST+x} ] && POSTGRESQL_HOST="localhost"
  [ -z ${POSTGRESQL_PORT+x} ] && POSTGRESQL_PORT="5432"
  [ -z ${POSTGRESQL_DATABASE+x} ] && POSTGRESQL_DATABASE="defaultdb"
  [ -z ${POSTGRESQL_USER+x} ] && POSTGRESQL_USER="test"
  [ -z ${POSTGRESQL_PASSWORD+x} ] && POSTGRESQL_PASSWORD="zaphod"
  [ -z ${POSTGRESQL_INSECURE+x} ] && POSTGRESQL_INSECURE="true"
  [ -z ${POSTGRESQL_IN_CONTAINER+x} ] && POSTGRESQL_IN_CONTAINER="false"
  [ -z ${POSTGRESQL_CONTAINER_NAME+x} ] && POSTGRESQL_CONTAINER_NAME="pgsql"
  FLAGS=()

  # handle flags
  FORCE=false
  VERBOSE=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force) FORCE=true ;;
      --verbose) VERBOSE=true ;;
      --help) usage; exit ;;
      *) FLAGS+=("$1")
    esac
    shift 1
  done

  FLAGS+=(-U "${POSTGRESQL_ROOT_USER}")
  FLAGS+=(--host "${POSTGRESQL_HOST}")
  FLAGS+=(--port "${POSTGRESQL_PORT}")

  # Useful for debugging
  FLAGS+=(--echo-all)

  # Optionally print flags (before appending password)
  [[ ${VERBOSE} = 'true' ]] && echo "- Using PostgreSQL Flags: ${FLAGS[@]}"

  # append password if supplied
  [ -z ${POSTGRESQL_ROOT_PASSWORD+x} ] || FLAGS+=(-p"${POSTGRESQL_ROOT_PASSWORD}")

  if [[ ${POSTGRESQL_IN_CONTAINER} = 'true' ]]; then
    CMD="docker exec -i ${POSTGRESQL_CONTAINER_NAME} psql"
  else
    CMD="psql"
  fi
}

main() {
  collect_vars "$@"

  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  echo "Warning: about to destroy and reset database '${POSTGRESQL_DATABASE}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      set -eux
      $CMD "${FLAGS[@]}" -c "DROP DATABASE IF EXISTS ${POSTGRESQL_DATABASE};" || \
        die "Error: Failed to drop database '${POSTGRESQL_DATABASE}'."
      $CMD "${FLAGS[@]}" -c "CREATE DATABASE ${POSTGRESQL_DATABASE};" || \
        die "Error: Failed to create database '${POSTGRESQL_DATABASE}'."
      if [[ ${POSTGRESQL_INSECURE} = 'true' ]]; then
        $CMD "${FLAGS[@]}" -c "CREATE USER ${POSTGRESQL_USER};" || \
          die "Error: Failed to create user '${POSTGRESQL_USER}'."
      else
        $CMD "${FLAGS[@]}" -c "CREATE USER ${POSTGRESQL_USER} WITH PASSWORD '${POSTGRESQL_PASSWORD}';" || \
          die "Error: Failed to create user '${POSTGRESQL_USER}'."
      fi
      $CMD "${FLAGS[@]}" -c "GRANT ALL PRIVILEGES ON DATABASE ${POSTGRESQL_DATABASE} TO ${POSTGRESQL_USER} WITH GRANT OPTION;" || \
        die "Error: Failed to grant '${POSTGRESQL_USER}' user all privileges on '${POSTGRESQL_DATABASE}'."
      $CMD "${FLAGS[@]}" -d ${POSTGRESQL_DATABASE} < ${TRILLIAN_PATH}/storage/postgresql/schema/storage.sql || \
        die "Error: Failed to create tables in '${POSTGRESQL_DATABASE}' database."
      echo "Reset Complete"
  fi
}

main "$@"
