#!/bin/bash

set -e

usage() {
  echo "$0 [--force] [--verbose] ..."
  echo "accepts environment variables:"
  echo " - MYSQL_USER"
  echo " - MYSQL_PASSWORD"
  echo " - MYSQL_HOST"
  echo " - MYSQL_PORT"
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${MYSQL_USER+x} ] && MYSQL_USER="root"
  [ -z ${MYSQL_NAME+x} ] && MYSQL_NAME="test"
  [ -z ${MYSQL_HOST+x} ] && MYSQL_HOST="localhost"
  [ -z ${MYSQL_PORT+x} ] && MYSQL_PORT="3306"
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

  FLAGS+=(-u "${MYSQL_USER}")
  FLAGS+=(--host "${MYSQL_HOST}")
  FLAGS+=(--port "${MYSQL_PORT}")

  # Optionally print flags (before appending password)
  [[ ${VERBOSE} = 'true' ]] && echo "- Using MySQL Flags: ${FLAGS[@]}"

  # append password if supplied
  [ -z ${MYSQL_PASSWORD+x} ] || FLAGS+=(-p"${MYSQL_PASSWORD}")
}

main() {
  collect_vars "$@"
  local CONN_LIMIT=1024

  echo "Warning: about to set MySQL global connection limit to ${CONN_LIMIT}"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      mysql "${FLAGS[@]}" -e "SET GLOBAL max_connections = ${CONN_LIMIT};"
      echo "Set Complete"
  fi
}

main "$@"
