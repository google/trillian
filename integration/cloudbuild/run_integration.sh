#!/bin/bash
set -e
set -x

export MYSQL_URI="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}"

# If the test will use etcd, then install etcd + tools.
if [ "${ETCD_DIR}" != "" ]; then
  go install go.etcd.io/etcd go.etcd.io/etcd/etcdctl github.com/fullstorydev/grpcurl/cmd/grpcurl
fi

./integration/integration_test.sh
./integration/maphammer.sh 3
