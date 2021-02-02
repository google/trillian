#!/bin/bash

set -ex

export MYSQL_HOST="${HOSTNAME}_db_1"
export MYSQL_PORT=$(( 10000 + $RANDOM % 30000))
export MYSQL_DATABASE="test"
export MYSQL_USER="test"
export MYSQL_PASSWORD="zaphod"
export MYSQL_ROOT_PASSWORD="bananas"
export MYSQL_USER_HOST="%"

docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml up -d
trap "docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml down" EXIT

# Wait for MySQL instance to be ready.
while ! mysql --protocol=TCP --host=${MYSQL_HOST} --port=${MYSQL_PORT} --user=root -pbananas -e quit ; do
 sleep 5
done

export TEST_MYSQL_URI="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}"

# If the test will use etcd, then install etcd + tools.
if [ "${ETCD_DIR}" != "" ]; then
  go install go.etcd.io/etcd go.etcd.io/etcd/etcdctl github.com/fullstorydev/grpcurl/cmd/grpcurl
fi

./integration/integration_test.sh
./integration/maphammer.sh 3
