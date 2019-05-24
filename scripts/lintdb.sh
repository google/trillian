#!/bin/bash

# Usage: lintdb.sh [branch] [SQL file]
#
# This tool lints and diffs the local copy of an SQL schema file against the
# version stored in the specified branch ("master" by default). If linting
# detects a problem or the diff identifies a backwards-incompatble change,
# the tool will exit with a non-zero exit code.

die() {
  echo $@ > /dev/stderr
  exit 1
}

schema_stashed=false

# Saves the given directory by stashing it if it contains uncommitted changes.
# Will automatically restore those changes on exit.
save_schema() {
  local schema_dir=${1:?Usage: save_schema <schema dir>}
  local output=$(git diff-index --exit-code HEAD -- ${schema_dir} 2>&1)
  case $? in
    0)
      # No uncommitted changes to database schema
      return 0
      ;;
    1)
      # Uncommitted changes to database schema - stash them
      git stash push --quiet -- ${schema_dir} ||
        die "Failed to stash ${schema_dir}"
      schema_stashed=true
      trap "restore_schema ${schema_dir}" EXIT
      ;;
    *)
      # Error occurred
      echo ${output}
      return 1
      ;;
  esac
}

# Restores the given directory from the current branch.
# If save_schema() stashed uncommitted changes, these will be restored.
restore_schema() {
  local schema_dir=${1:?Usage: restore_schema <schema dir>}

  # Restore the schema from this branch. It is necessary to reset $schema_dir
  # before checking it out, because the checkout will have added it to the index
  # (resetting it undoes this, allowing the checkout to succeed).
  git reset -q ${schema_dir} && git checkout -- ${schema_dir} ||
    die "Failed to checkout ${schema_dir}"

  if ${schema_stashed}; then
    git stash pop --quiet ||
      die "Failed to restore ${schema_dir} from Git stash"
    schema_stashed=false
  fi
}

configure_skeema() {
  local skeema_file=${1:?Usage: configure_skeema <schema dir>}/.skeema
  cat << EOF >> ${skeema_file} || die "Failed to configure Skeema"

[${diff_schema}]
host=127.0.0.1
port=3306
schema=${diff_schema}
new-schemas=false
EOF
}

main() {
  local schema_dir=${1:-storage/mysql/schema}
  local diff_branch=${2:-master}

  git diff --quiet ${diff_branch} -- ${schema_dir}
  case $? in
    0)
      # No changes made to database schema
      return 0
      ;;
    1)
      # Changes made - diff them.
      ;;
    *)
      # Error occurred
      return 1
      ;;
  esac

  local diff_hash=$(git rev-parse --short ${diff_branch})
  local diff_schema="trillian_${diff_hash}"

  echo "Creating ${diff_schema} database based on schema from ${diff_branch}..."

  # Save the current schema, then checkout the version from $diff_branch.
  save_schema "${schema_dir}" || die "Failed to save ${schema_dir}"
  git checkout ${diff_branch} -- ${schema_dir} ||
    die "Failed to checkout ${schema_dir} from ${diff_branch}"

  # Get Skeema to create a database from the checked out schema.
  configure_skeema ${schema_dir} || die "Failed to configure Skeema"
  (cd ${schema_dir} && skeema push ${diff_schema}) ||
    die "Failed to create database from ${schema_dir} in ${diff_branch} branch"

  echo "Diffing current schema against ${diff_schema}..."

  # Restore the schema to its original state.
  restore_schema ${schema_dir} || die "Failed to restore ${schema_dir}"

  # Have to reconfigure Skeema because restore_schema() will have erased
  # changes made by the last call to configure_skeema().
  configure_skeema ${schema_dir} || die "Failed to reconfigure Skeema"
  # Lint and diff the original schema against the version from $diff_branch.
  (cd ${schema_dir} && skeema lint ${diff_schema}) || die "${schema_dir} failed lint check"
  (cd ${schema_dir} && skeema diff ${diff_schema})
  # An exit code > 1 means an error occurred or an incompatible change was
  # detected in the schema. 0 means no diff, 1 means diff.
  if [[ $? > 1 ]]; then
    die
  fi
}

main "$@"
