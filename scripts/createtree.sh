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
    VALUES (${tree_id}, 'ACTIVE', '${tree_type}', 'RFC_6962', 'SHA256', 'RSA', 'NOT_ALLOWED', NOW(), NOW(), X'0a27747970652e676f6f676c65617069732e636f6d2f7472696c6c69616e2e50454d4b657946696c65122c0a2374657374646174612f6c6f672d7270632d7365727665722e707269766b65792e70656d1205746f77656c')"
mysql ${testdbopts} -e "
    INSERT INTO TreeControl(TreeId, SigningEnabled, SequencingEnabled, SequenceIntervalSeconds)
    VALUES(${tree_id}, true, true, 1)"
