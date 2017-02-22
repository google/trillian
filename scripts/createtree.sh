#!/bin/sh
set -eu

if [ $# -ne 2 ]; then
    echo "Usage: $0 <treeid> <treetype>"
    exit 1
fi

testdbopts='-u test --password=zaphod -D test'
tree_id=$1
tree_type=$2

mysql ${testdbopts} -e "
    INSERT INTO Trees(TreeId, TreeState, TreeType, HashStrategy, HashAlgorithm, SignatureAlgorithm, DuplicatePolicy, CreateTime, UpdateTime)
    VALUES (${tree_id}, 'ACTIVE', '${tree_type}', 'RFC_6962', 'SHA256', 'RSA', 'NOT_ALLOWED', NOW(), NOW())"
mysql ${testdbopts} -e "
    INSERT INTO TreeControl(TreeId, SigningEnabled, SequencingEnabled, SequenceIntervalSeconds)
    VALUES(${tree_id}, true, true, 1)"
