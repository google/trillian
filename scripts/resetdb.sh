#!/bin/bash

set -e

usage() {
  echo "$0 [--force] [--verbose] ..."
  echo "accepts environment variables:"
  echo " - DB_NAME"
  echo " - DB_USER"
  echo " - DB_PASSWORD"
}

collect_vars() {
  # set unset environment variables to defaults
  [ -z ${DB_USER+x} ] && DB_USER="root"
  [ -z ${DB_NAME+x} ] && DB_NAME="test"
  # format reused supplied environment variables
  FLAGS=""
  [ -z ${DB_PASSWORD+x} ] || FLAGS="${FLAGS} -p$DB_PASSWORD"

  # handle flags
  FORCE=false
  VERBOSE=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force) FORCE=true ;;
      --verbose) VERBOSE=true ;;
      *) FLAGS="${FLAGS} $1"
    esac
    shift 1
  done
}

main() {
  collect_vars "$@"

  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  # what we're about to do
  if [[ ${VERBOSE} = 'true' ]]
  then
    echo "-- using DB_USER: ${DB_USER}"
  fi
  echo "Warning: about to destroy and reset database '${DB_NAME}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? " -n 1 -r

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      mysql -u $DB_USER $FLAGS -e "DROP DATABASE IF EXISTS ${DB_NAME};"
      mysql -u $DB_USER $FLAGS -e "CREATE DATABASE ${DB_NAME};"
      mysql -u $DB_USER $FLAGS -e "GRANT ALL ON ${DB_NAME}.* TO '${DB_NAME}'@'localhost' IDENTIFIED BY 'zaphod';"
      mysql -u $DB_USER $FLAGS -D ${DB_NAME} < ${TRILLIAN_PATH}/storage/mysql/storage.sql
      echo "Reset Complete"
  fi
}

main "$@"
