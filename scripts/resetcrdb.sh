#!/bin/bash

set -e

usage() {
  cat <<EOF
$(basename $0) [--force] [--verbose] ...
All unrecognised arguments will be passed through to the 'cockroach' command.
Accepts environment variables:
- CRDB_ROOT_USER: A user with sufficient rights to create/reset the Trillian
  database (default: root).
- CRDB_ROOT_PASSWORD: The password for \$CRDB_ROOT_USER (default: none).
- CRDB_HOST: The hostname of the MySQL server (default: localhost).
- CRDB_PORT: The port the MySQL server is listening on (default: 3306).
- CRDB_DATABASE: The name to give to the new Trillian user and database
  (default: test).
- CRDB_USER: The name to give to the new Trillian user (default: test).
- CRDB_PASSWORD: The password to use for the new Trillian user
  (default: zaphod).
- CRDB_USER_HOST: The host that the Trillian user will connect from; use '%' as
  a wildcard (default: localhost).
- CRDB_IN_CONTAINER: If set, the script will assume it is running in a Docker
  container and will exec into the container to operate.
- CRDB_CONTAINER_NAME: The name of the Docker container to exec into (default:
  roach).
EOF
}

die() {
  echo "$*" > /dev/stderr
  exit 1
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${CRDB_ROOT_USER+x} ] && CRDB_ROOT_USER="root"
  [ -z ${CRDB_HOST+x} ] && CRDB_HOST="localhost"
  [ -z ${CRDB_PORT+x} ] && CRDB_PORT="26257"
  [ -z ${CRDB_DATABASE+x} ] && CRDB_DATABASE="defaultdb"
  [ -z ${CRDB_USER+x} ] && CRDB_USER="test"
  [ -z ${CRDB_PASSWORD+x} ] && CRDB_PASSWORD="zaphod"
  [ -z ${CRDB_USER_HOST+x} ] && CRDB_USER_HOST="localhost"
  [ -z ${CRDB_INSECURE+x} ] && CRDB_INSECURE="true"
  [ -z ${CRDB_IN_CONTAINER+x} ] && CRDB_IN_CONTAINER="false"
  [ -z ${CRDB_CONTAINER_NAME+x} ] && CRDB_CONTAINER_NAME="roach"
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

  FLAGS+=(-u "${CRDB_ROOT_USER}")
  FLAGS+=(--host "${CRDB_HOST}")
  FLAGS+=(--port "${CRDB_PORT}")

  # Useful for debugging
  FLAGS+=(--echo-sql)

  if [[ ${CRDB_INSECURE} = 'true' ]]; then
    FLAGS+=(--insecure)
  fi

  # Optionally print flags (before appending password)
  [[ ${VERBOSE} = 'true' ]] && echo "- Using MySQL Flags: ${FLAGS[@]}"

  # append password if supplied
  [ -z ${CRDB_ROOT_PASSWORD+x} ] || FLAGS+=(-p"${CRDB_ROOT_PASSWORD}")

  if [[ ${CRDB_IN_CONTAINER} = 'true' ]]; then
    CMD="docker exec -i ${CRDB_CONTAINER_NAME} cockroach"
  else
    CMD="cockroach"
  fi
}

main() {
  collect_vars "$@"

  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  echo "Warning: about to destroy and reset database '${CRDB_DATABASE}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      set -eux
      $CMD sql "${FLAGS[@]}" -e "DROP DATABASE IF EXISTS ${CRDB_DATABASE};" || \
        die "Error: Failed to drop database '${CRDB_DATABASE}'."
      $CMD sql "${FLAGS[@]}" -e "CREATE DATABASE ${CRDB_DATABASE};" || \
        die "Error: Failed to create database '${CRDB_DATABASE}'."
      if [[ ${CRDB_INSECURE} = 'true' ]]; then
        $CMD sql "${FLAGS[@]}" -e "CREATE USER IF NOT EXISTS ${CRDB_USER};" || \
          die "Error: Failed to create user '${CRDB_USER}'."
      else
        $CMD sql "${FLAGS[@]}" -e "CREATE USER IF NOT EXISTS ${CRDB_USER} WITH PASSWORD '${CRDB_PASSWORD}';" || \
          die "Error: Failed to create user '${CRDB_USER}'."
      fi
      $CMD sql "${FLAGS[@]}" -e "GRANT ALL PRIVILEGES ON DATABASE ${CRDB_DATABASE} TO ${CRDB_USER} WITH GRANT OPTION" || \
        die "Error: Failed to grant '${CRDB_USER}' user all privileges on '${CRDB_DATABASE}'."
      $CMD sql "${FLAGS[@]}" -d ${CRDB_DATABASE} < ${TRILLIAN_PATH}/storage/crdb/schema/storage.sql || \
        die "Error: Failed to create tables in '${CRDB_DATABASE}' database."
      echo "Reset Complete"
  fi
}

main "$@"
