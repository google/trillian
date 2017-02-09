#!/bin/sh
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <logid>"
fi
TESTDBOPTS="-u test --password=zaphod -D test"
TREE_ID=$1
# Create a new Map storage row for the given tree ID.
mysql ${TESTDBOPTS} -e "INSERT INTO Trees(TreeId, KeyId, TreeType, LeafHasherType, TreeHasherType, AllowsDuplicateLeaves) VALUES(${TREE_ID}, 1, 'MAP', 'SHA256', 'SHA256', false)"
