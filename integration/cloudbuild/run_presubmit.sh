#!/bin/bash

set -ex

# Use the default MySQL port. There is no need to override it because the
# docker-compose config has only one MySQL instance, and ${MYSQL_HOST} uniquely
# identifies it.
MYSQL_PORT=3306
export MYSQL_HOST="${HOSTNAME}_db_1"

docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml up -d
trap "docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml down" EXIT

# Wait for MySQL instance to be ready.
while ! mysql --protocol=TCP --host=${MYSQL_HOST} --port=${MYSQL_PORT} --user=root -pbananas \
  -e 'SHOW VARIABLES LIKE "%version%";' ;
do
 sleep 5
done

# Presumbits need a user with CREATE DATABASE grants since they create temporary
# databases. For the same reason, this is a URI prefix - tests will add DB names
# to the end.
export TEST_MYSQL_URI="root:bananas@tcp(${MYSQL_HOST}:${MYSQL_PORT})/"

./scripts/presubmit.sh $*

# TODO(pavelkalinnikov): Make the check more robust.
if [ $1 = "--coverage" ]; then
  bash <(curl -s https://codecov.io/bash)
fi
