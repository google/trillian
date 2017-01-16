#!/bin/bash
# Builds configuration for a CT test, set in ${CT_CFG},
# sets ${TREE_IDS} and provisions logs for them.
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

# Build config file with absolute paths
CT_CFG=$(mktemp "${INTEGRATION_DIR}"/ct-XXXXXX)
trap "rm ${CT_CFG}" EXIT
sed "s!@TESTDATA@!${TESTDATA}!" ./integration/ct_integration_test.cfg > ${CT_CFG}

# Retrieve tree IDs from config file
TREE_IDS=$(grep LogID ${CT_CFG} | grep -o '[0-9]\+')
for id in ${TREE_IDS}
do
    echo "Provisioning test log (Tree ID: ${id}) in database"
    "${SCRIPTS_DIR}"/wipelog.sh ${id}
    "${SCRIPTS_DIR}"/createlog.sh ${id}
done

