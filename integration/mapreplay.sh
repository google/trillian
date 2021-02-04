#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

MAP_JOURNAL=${1}
if [[ ! -f "${MAP_JOURNAL}" ]]; then
  echo "First argument should be map journal file"
  exit 1
fi

declare -a OLD_MAP_ARRAY
OLD_MAP_IDS=$(go run github.com/google/trillian/testonly/internal/hammer/mapreplay \
  --logtostderr -v 2 --replay_from ${MAP_JOURNAL} 2>&1 >/dev/null | grep "map_id:" | sed 's/.*map_id:\([0-9]\+\).*/\1/' | sort | uniq)
for mapid in "${OLD_MAP_IDS}"; do
  OLD_MAP_ARRAY+=(${mapid})
done
MAP_COUNT=${#OLD_MAP_ARRAY[@]}
echo "Observed map IDs in ${MAP_JOURNAL}: ${OLD_MAP_IDS}"

map_prep_test 1
TO_KILL+=(${RPC_SERVER_PIDS[@]})

echo "Provisioning ${MAP_COUNT} map(s)"
map_provision "${RPC_SERVER_1}" ${MAP_COUNT}
echo "Provisioned ${MAP_COUNT} map(s): ${MAP_IDS}"

declare -a MAP_ARRAY
for mapid in $(echo ${MAP_IDS} | sed 's/,/ / '); do
  MAP_ARRAY+=(${mapid})
done

# Map from original map IDs to the newly provisioned ones.
MAPMAP=""
for ((i=0; i < MAP_COUNT; i++)); do
  if [[ $i -eq 0 ]]; then
    MAPMAP="${OLD_MAP_ARRAY[$i]}:${MAP_ARRAY[$i]}"
  else
    MAPMAP="${MAPMAP},${OLD_MAP_ARRAY[$i]}:${MAP_ARRAY[$i]}"
  fi
done
echo "Mapping map/tree IDs ${MAPMAP}"

echo "Replaying requests from ${MAP_JOURNAL}"
set +e
go run github.com/google/trillian/testonly/internal/hammer/mapreplay \
  --map_ids=${MAPMAP} --rpc_server=${RPC_SERVER_1} --logtostderr -v 1 --replay_from "${MAP_JOURNAL}"
RESULT=$?
set -e

map_stop_test
TO_KILL=()

exit $RESULT
