#!/bin/bash

set -e

usage() {
  echo "$0 [--force] [--verbose] ..."
  echo "accepts environment variables:"
  echo " - DB_NAME"
  echo " - DB_USER"
  echo " - DB_PASSWORD"
  echo " - DB_HOST"
  echo " - DB_PORT"
}

die() {
  echo "$*" > /dev/stderr
  exit 1
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

  readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

  # what we're about to do
  echo "Warning: about to destroy and reset database '${DB_NAME}'"

  [[ ${FORCE} = true ]] || read -p "Are you sure? [Y/N]: " -n 1 -r
  echo # Print newline following the above prompt

  if [ -z ${REPLY+x} ] || [[ $REPLY =~ ^[Yy]$ ]]
  then
      echo "Resetting DB..."
      mysql "${FLAGS[@]}" -e "DROP DATABASE IF EXISTS ${DB_NAME};" || \
        die "Error: Failed to drop database."
      mysql "${FLAGS[@]}" -e "CREATE DATABASE ${DB_NAME};" || \
        die "Error: Failed to create database."
      mysql "${FLAGS[@]}" -e "CREATE USER IF NOT EXISTS ${DB_NAME}@'localhost' IDENTIFIED BY 'zaphod';" || \
        die "Error: Failed to create user."
      mysql "${FLAGS[@]}" -e "GRANT ALL ON ${DB_NAME}.* TO ${DB_NAME}@'localhost'" || \
        die "Error: Failed to grant user privileges."
      mysql "${FLAGS[@]}" -D ${DB_NAME} < ${TRILLIAN_PATH}/storage/mysql/storage.sql || \
        die "Error: Failed to create tables."
      echo "Reset Complete"
  fi
}

main "$@"
