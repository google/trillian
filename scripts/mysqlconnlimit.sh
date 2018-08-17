#!/bin/bash

set -e

usage() {
  echo "$0 [--force] [--verbose] ..."
  echo "accepts environment variables:"
  echo " - DB_USER"
  echo " - DB_PASSWORD"
  echo " - DB_HOST"
  echo " - DB_PORT"
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${DB_USER+x} ] && DB_USER="root"
  [ -z ${DB_NAME+x} ] && DB_NAME="test"
  [ -z ${DB_HOST+x} ] && DB_HOST="localhost"
  [ -z ${DB_PORT+x} ] && DB_PORT="3306"
  FLAGS=()

  # handle flags
  FORCE=false
  VERBOSE=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force) FORCE=true ;;
      --verbose) VERBOSE=true ;;
      *) FLAGS+=("$1")
    esac
    shift 1
  done

  FLAGS+=(-u "${DB_USER}")
  FLAGS+=(--host "${DB_HOST}")
  FLAGS+=(--port "${DB_PORT}")

  # Optionally print flags (before appending password)
  [[ ${VERBOSE} = 'true' ]] && echo "- Using MySQL Flags: ${FLAGS[@]}"

  # append password if supplied
  [ -z ${DB_PASSWORD+x} ] || FLAGS+=(-p"${DB_PASSWORD}")
}

main() {
  collect_vars "$@"
  CONN_LIMIT=1024

  # what we're about to do
  echo "Warning: about set MySQL global connection limit to ${CONN_LIMIT}"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Setting connection limit to ${CONN_LIMIT}..."
      mysql "${FLAGS[@]}" -e "SET GLOBAL max_connections = ${CONN_LIMIT};"
      echo "Set Complete"
  fi
}

main "$@"
