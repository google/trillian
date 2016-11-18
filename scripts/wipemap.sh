#!/bin/sh
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <logid>"
fi
TESTDBOPTS="-u test --password=zaphod -D test"
TREE_ID=$1
# Wipe all Map storage rows for the given tree ID.
mysql ${TESTDBOPTS} -e "DELETE FROM Trees WHERE TreeId = ${TREE_ID}"
