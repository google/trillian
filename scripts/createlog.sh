#!/bin/sh
set -eu

if [ $# -ne 1 ]; then
    echo "Usage: $0 <logid>"
fi

TESTDBOPTS='-u test --password=zaphod -D test'
TREE_ID=$1

# Create a new Log storage row for the given tree ID.
mysql ${TESTDBOPTS} -e "INSERT INTO Trees VALUES (${TREE_ID}, 1, 'LOG', 'SHA256', 'SHA256', false)"
mysql ${TESTDBOPTS} -e "INSERT INTO TreeControl VALUES (${TREE_ID},false,false,false,1,1)"
