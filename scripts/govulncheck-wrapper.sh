#!/bin/bash
# This script wraps govulncheck to allow ignoring specific vulnerabilities.
#
# Problem Solved:
#   govulncheck does not natively support ignoring/silencing specific CVEs/vulnerabilities.
#
# How it works:
#   1. Runs `govulncheck -json` to get detailed finding data.
#   2. Parses `.govulncheck-ignore` to get the list of ignored vuln IDs + modules.
#   3. Filters the findings.
#   4. Crucially, it ONLY ignores a vulnerability if there is NO fix available yet.
#      If a vulnerability is in the ignore list but has a 'fixed_version' reported
#      by govulncheck, the script will NOT ignore it and will fail the build,
#      forcing an upgrade when possible.
set -euo pipefail

CONFIG_FILE=".govulncheck-ignore"
VERBOSE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      CONFIG_FILE="$2"
      shift 2
      ;;
    --verbose)
      VERBOSE=1
      shift
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

log_info() {
  echo "[INFO] $*"
}

log_error() {
  echo "[ERROR] $*" >&2
}

if ! command -v govulncheck &> /dev/null; then
  log_error "govulncheck not found."
  exit 1
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
  log_error "Config file not found: $CONFIG_FILE"
  exit 1
fi

if ! command -v jq &> /dev/null; then
  log_error "jq not found."
  exit 1
fi

[[ $VERBOSE -eq 1 ]] && log_info "Running govulncheck..."
set +e
VULN_JSON=$(govulncheck -json ./...)
GOVULNCHECK_EXIT=$?
set -e

if [[ $GOVULNCHECK_EXIT -ne 0 && $GOVULNCHECK_EXIT -ne 3 ]]; then
  log_error "govulncheck failed (exit $GOVULNCHECK_EXIT)"
  exit 1
fi

if [[ -n "$VULN_JSON" ]]; then
  if ! echo "$VULN_JSON" | jq -e -n 'inputs' 2>&1 >/dev/null; then
    log_error "govulncheck output is not valid JSON"
    echo "$VULN_JSON" >&2
    exit 1
  fi
fi

UNIQUE_VULNS_TSV=$(echo "$VULN_JSON" | jq -r -s '
  [ .[] 
    | select(.finding) 
    | select(.finding.trace | length > 1) 
    | {
        id: .finding.osv, 
        module: .finding.trace[0].module, 
        fixed: (if .finding.fixed_version and .finding.fixed_version != "" then .finding.fixed_version else "N/A" end)
      }
  ] 
  | unique_by(.id + .module) 
  | .[] 
  | "\(.id)\t\(.module)\t\(.fixed)"
' 2>/dev/null || true)

if [[ -z "$UNIQUE_VULNS_TSV" ]]; then
  log_info "No vulnerabilities found"
  exit 0
fi

IGNORED_LIST=""
while read -r id module reason || [[ -n "$id" ]]; do
  # Skip comments and empty lines
  if [[ -z "$id" || "$id" =~ '^#' ]]; then
    continue
  fi
  if [[ -n "$id" && -n "$module" ]]; then
    IGNORED_LIST="${IGNORED_LIST}${id}|${module}"$'\n'
  fi
done < "$CONFIG_FILE"

IGNORED_COUNT=0
UNIGNORED_COUNT=0
UNIGNORED_VULNS=""

while IFS=$'\t' read -r vuln_id module fixed; do
  [[ -z "$vuln_id" ]] && continue

  if grep -qxF "${vuln_id}|${module}" <<< "$IGNORED_LIST"; then
    if [[ "$fixed" == "N/A" ]]; then
      ((IGNORED_COUNT++)) || true
    else
      ((UNIGNORED_COUNT++)) || true
      UNIGNORED_VULNS="${UNIGNORED_VULNS}  - $vuln_id in $module (fix available: $fixed)\n"
    fi
  else
    ((UNIGNORED_COUNT++)) || true
    UNIGNORED_VULNS="${UNIGNORED_VULNS}  - $vuln_id in $module (fixed: $fixed)\n"
  fi
done <<< "$UNIQUE_VULNS_TSV"

log_info "Found $UNIGNORED_COUNT unignored vulnerabilities, $IGNORED_COUNT ignored"

if [[ $UNIGNORED_COUNT -gt 0 ]]; then
  log_error "Unignored vulnerabilities found:"
  echo -e "$UNIGNORED_VULNS" >&2
  exit 1
fi

exit 0
