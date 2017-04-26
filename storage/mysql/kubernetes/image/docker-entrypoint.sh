#!/bin/bash

# Original work: Copyright 2016 The Kubernetes Authors.
# Modified work: Copyright 2017 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.



# This script does the following:
#
# 1. If starting the first replica in the cluster, and MySQL has not been
# initialized, creates the database according to the following environment
# variables:
# - $MYSQL_ROOT_PASSWORD
# - $MYSQL_DATABASE
# - $MYSQL_USER
# - $MYSQL_PASSWORD
# - $WSREP_SST_USER
# - $WSREP_SST_PASSWORD
# 2. Configures MySQL for the Galera cluster.

set -e

if [ "${1:0:1}" = '-' ]; then
  set -- mysqld "$@"
fi

# The MySQL "data_dir", where the databases are stored.
readonly DATADIR="/var/lib/mysql"
# Make sure that it exists and is owned by the MySQL user and group.
mkdir -p "$DATADIR"
chown -R mysql:mysql "$DATADIR"

# If this is the first node, it may be necessary to initialize the database.
# All other nodes will replicate this database.
if [[ "$(hostname)" == *-0 ]]; then
  # Check whether the "mysql" database exists. If it doesn't, MySQL can't have
  # been initialized yet.
  if [ ! -d "$DATADIR/mysql" ]; then
    echo "No \"mysql\" database found in $DATADIR - initializing..."

    if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
      echo >&2 'error: MYSQL_ROOT_PASSWORD not set'
      exit 1
    fi

    # Create an SQL script that sets up users, permissions and databases.
    INIT_SQL=$(mktemp)

    # Create root user.
    cat > "$INIT_SQL" <<-EOSQL
DELETE FROM mysql.user ;
CREATE USER 'root'@'%' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}' ;
GRANT ALL ON *.* TO 'root'@'%' WITH GRANT OPTION ;
EOSQL

    # Create database.
    if [ "$MYSQL_DATABASE" ]; then
      echo "CREATE DATABASE IF NOT EXISTS \`$MYSQL_DATABASE\` ;" >> "$INIT_SQL"
    fi

    # Create a user. If a database was created, grant this user all permissions
    # on it.
    if [ "$MYSQL_USER" -a "$MYSQL_PASSWORD" ]; then
      echo "CREATE USER '$MYSQL_USER'@'%' IDENTIFIED BY '$MYSQL_PASSWORD' ;" >> "$INIT_SQL"

      if [ "$MYSQL_DATABASE" ]; then
        echo "GRANT ALL ON \`$MYSQL_DATABASE\`.* TO '$MYSQL_USER'@'%' ;" >> "$INIT_SQL"
      fi
    fi

    # Create a user with no password or permissions that can be used for things
    # like health checks.
    echo "CREATE USER 'dummy'@'localhost';" >> "$INIT_SQL"

    # Set up a user for state transfer (SST), as required by Galera:
    # http://galeracluster.com/documentation-webpages/statetransfer.html

    WSREP_SST_USER=${WSREP_SST_USER:-"sst"}
    if [ -z "$WSREP_SST_PASSWORD" ]; then
      echo >&2 'error: WSREP_SST_PASSWORD is not set'
      exit 1
    fi

    echo "CREATE USER '${WSREP_SST_USER}'@'localhost' IDENTIFIED BY '${WSREP_SST_PASSWORD}';" >> "$INIT_SQL"
    echo "GRANT PROCESS, RELOAD, LOCK TABLES, REPLICATION CLIENT ON *.* TO '${WSREP_SST_USER}'@'localhost';" >> "$INIT_SQL"

    echo 'FLUSH PRIVILEGES ;' >> "$INIT_SQL"

    # Initialize MySQL using the script that was just assembled.
    mysqld --initialize --datadir "$DATADIR" --init-file "$INIT_SQL" --ignore-db-dir="lost+found"
    rm "$INIT_SQL"
  fi
fi

WSREP_SST_USER=${WSREP_SST_USER:-"sst"}
if [ -z "$WSREP_SST_PASSWORD" ]; then
  echo >&2 'error: WSREP_SST_PASSWORD not set'
  exit 1
fi

# Configure MySQL to use the Galera cluster.

# Provide the SST user and password.
sed -i -e "s|^wsrep_sst_auth=.*$|wsrep_sst_auth=\"${WSREP_SST_USER}:${WSREP_SST_PASSWORD}\"|" /etc/mysql/conf.d/cluster.cnf

# Provide the replica's own IP address.
WSREP_NODE_ADDRESS=`ip addr show | grep -E '^[ ]*inet' | grep -m1 global | awk '{ print $2 }' | sed -e 's/\/.*//'`
if [ -n "$WSREP_NODE_ADDRESS" ]; then
  sed -i -e "s|^wsrep_node_address=.*$|wsrep_node_address=${WSREP_NODE_ADDRESS}|" /etc/mysql/conf.d/cluster.cnf
fi

# All but the first replica should be given the address of the first replca in
# order to join the cluster. The first replica will boostrap the cluster.
if [[ "$(hostname)" != *-0 ]]; then
  sed -i -e "s|^wsrep_cluster_address=gcomm://|wsrep_cluster_address=${WSREP_CLUSTER_ADDRESS}|" /etc/mysql/conf.d/cluster.cnf
fi

# Provide a random server ID for this replica.
sed -i -e "s/^server\-id=.*$/server-id=${RANDOM}/" /etc/mysql/my.cnf

# Finally, start MySQL, passing through any flags.
exec "$@" --ignore-db-dir="lost+found"

