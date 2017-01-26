#!/bin/sh
set -eu

if [ $# -ne 1 ]; then
    echo "Usage: $0 <logid>"
fi

TESTDBOPTS='-u test --password=zaphod -D test'
TREE_ID=$1

# Wipe all Log storage rows for the given tree ID.
mysql ${TESTDBOPTS} -e "DELETE FROM Unsequenced WHERE TreeId = ${TREE_ID}"
mysql ${TESTDBOPTS} -e "DELETE FROM TreeHead WHERE TreeId = ${TREE_ID}"
mysql ${TESTDBOPTS} -e "DELETE FROM SequencedLeafData WHERE TreeId = ${TREE_ID}"
mysql ${TESTDBOPTS} -e "DELETE FROM LeafData WHERE TreeId = ${TREE_ID}"
mysql ${TESTDBOPTS} -e "DELETE FROM TreeControl WHERE TreeId = ${TREE_ID}"
mysql ${TESTDBOPTS} -e "DELETE FROM Trees WHERE TreeId = ${TREE_ID}"
