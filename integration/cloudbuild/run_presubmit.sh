#!/bin/bash
set -e
set -x

export MYSQL_PORT=$((10000 + $RANDOM % 30000))
export MYSQL_HOST="${HOSTNAME}_db_1"

docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml up -d
trap "docker-compose -p ${HOSTNAME} -f ./integration/cloudbuild/docker-compose-mysql.yaml down" EXIT

# Wait for mysql instance to be ready
while ! mysql --protocol=TCP --host=${MYSQL_HOST} --port=${MYSQL_PORT} --user=root -pbananas -e quit ; do
 sleep 1
done

# Presumbits need a user with CREATE DATABASE grants since they create temporary databases.
# For the same reason, this is a URI prefix - tests will add DB names to the end.
export MYSQL_URI="root:bananas@tcp(${MYSQL_HOST}:${MYSQL_PORT})/"

./scripts/presubmit.sh $*
