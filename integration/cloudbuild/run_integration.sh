#!/bin/bash
set -e
set -x

export MYSQL_URI="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}"

# If the test will use etcd, then install etcd + tools.
if [ "${ETCD_DIR}" != "" ]; then
  go get -v go.etcd.io/etcd/v3@v3.0.0-20210107172604-c632042bb96c
  go get -v go.etcd.io/etcd/etcdctl/v3@v3.0.0-20210107172604-c632042bb96c
  go install github.com/fullstorydev/grpcurl/cmd/grpcurl
fi

./integration/integration_test.sh
./integration/maphammer.sh 3
