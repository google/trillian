#!/bin/bash

set -ex

# Use the default MySQL port. There is no need to override it because the
# docker-compose config has only one MySQL instance, and ${MYSQL_HOST} uniquely
# identifies it.
MYSQL_PORT=3306
export MYSQL_HOST="${HOSTNAME}_db_1"
export MYSQL_DATABASE="test"
export MYSQL_USER="test"
export MYSQL_PASSWORD="zaphod"
export MYSQL_ROOT_PASSWORD="bananas"
export MYSQL_USER_HOST="%"


# See: https://docs.docker.com/compose/extends/#multiple-compose-files.
COMPOSE_CONFIG="-p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose.mysql.yml -f ./integration/cloudbuild/docker-compose.network.yml"
docker-compose $COMPOSE_CONFIG up -d
trap "docker-compose $COMPOSE_CONFIG down" EXIT

# Wait for MySQL instance to be ready.
while ! mysql --protocol=TCP --host=${MYSQL_HOST} --port=${MYSQL_PORT} --user=root -pbananas \
  -e 'SHOW VARIABLES LIKE "%version%";' ;
do
 sleep 5
done

export TEST_MYSQL_URI="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}"

# If the test will use etcd, then install etcd + tools.
if [ "${ETCD_DIR}" != "" ]; then
  go install go.etcd.io/etcd go.etcd.io/etcd/etcdctl github.com/fullstorydev/grpcurl/cmd/grpcurl
fi

go test -alsologtostderr ./storage/mysql/...

./integration/integration_test.sh
./integration/maphammer.sh 3
