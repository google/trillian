#!/bin/bash
set -e
set -x


# Presumbits need a user with CREATE DATABASE grants since they create temporary databases.
# For the same reason, this is a URI prefix - tests will add DB names to the end.
export MYSQL_URI="root:${MYSQL_ROOT_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/"

./scripts/presubmit.sh $*
