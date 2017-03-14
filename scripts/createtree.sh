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
    INSERT INTO Trees(TreeId, TreeState, TreeType, HashStrategy, HashAlgorithm, SignatureAlgorithm, DuplicatePolicy, CreateTime, UpdateTime, PrivateKey)
    VALUES (${tree_id}, 'ACTIVE', '${tree_type}', 'RFC_6962', 'SHA256', 'RSA', 'NOT_ALLOWED', NOW(), NOW(), '\x0a\x27\x74\x79\x70\x65\x2e\x67\x6f\x6f\x67\x6c\x65\x61\x70\x69\x73\x2e\x63\x6f\x6d\x2f\x74\x72\x69\x6c\x6c\x69\x61\x6e\x2e\x50\x45\x4d\x4b\x65\x79\x46\x69\x6c\x65\x12\x2c\x0a\x23\x74\x65\x73\x74\x64\x61\x74\x61\x2f\x6c\x6f\x67\x2d\x72\x70\x63\x2d\x73\x65\x72\x76\x65\x72\x2e\x70\x72\x69\x76\x6b\x65\x79\x2e\x70\x65\x6d\x12\x05\x74\x6f\x77\x65\x6c')"
mysql ${testdbopts} -e "
    INSERT INTO TreeControl(TreeId, SigningEnabled, SequencingEnabled, SequenceIntervalSeconds)
    VALUES(${tree_id}, true, true, 1)"
