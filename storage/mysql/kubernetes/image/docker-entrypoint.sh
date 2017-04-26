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

# The MySQL "datadir", where the databases are stored.
readonly DATADIR="/var/lib/mysql"

if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
  echo >&2 'error: MYSQL_ROOT_PASSWORD not set'
  exit 1
fi

if [ -z "$WSREP_SST_USER" -o -z "$WSREP_SST_PASSWORD" ]; then
  echo >&2 'error: WSREP_SST_USER or WSREP_SST_PASSWORD is not set'
  exit 1
fi

# Make sure that the datadir exists and is owned by the MySQL user and group.
mkdir -p "$DATADIR"
chown -R mysql:mysql "$DATADIR"

# If this is the first node, initialize the mysql database if it does not exist.
# This database will be replicated to all other nodes via SST.
if [[ "$(hostname)" == *-0 ]]; then
  if [ ! -d "${DATADIR}/mysql" ]; then
    mysqld --initialize --user=mysql --datadir "${DATADIR}" --ignore-db-dir "lost+found"
  fi
fi

# This SQL script will be run when the server starts up.
INIT_SQL=$(mktemp)
chmod 0600 "${INIT_SQL}"

# Create the following users:
# - root user for administrative purposes.
# - dummy user with no password or rights, for use by health checks.
# - SST user for use by Galera to replicate database state between nodes.
# TODO(robpercival): Restrict root access.
cat > "$INIT_SQL" <<EOSQL
DELETE FROM mysql.user;
CREATE USER 'root'@'%' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';
GRANT ALL ON *.* TO 'root'@'%' WITH GRANT OPTION;

CREATE USER 'dummy'@'localhost';

CREATE USER '${WSREP_SST_USER}'@'localhost' IDENTIFIED BY '${WSREP_SST_PASSWORD}';
GRANT PROCESS, RELOAD, LOCK TABLES, REPLICATION CLIENT ON *.* TO '${WSREP_SST_USER}'@'localhost';
FLUSH PRIVILEGES;
EOSQL

# Provide the SST user and password.
sed -i -e "s|^wsrep_sst_auth=.*$|wsrep_sst_auth=\"${WSREP_SST_USER}:${WSREP_SST_PASSWORD}\"|" /etc/mysql/conf.d/cluster.cnf

# Provide the replica's own IP address.
WSREP_NODE_ADDRESS=`ip addr show | grep -E '^[ ]*inet' | grep -m1 global | awk '{ print $2 }' | sed -e 's/\/.*//'`
if [ -n "$WSREP_NODE_ADDRESS" ]; then
  sed -i -e "s|^wsrep_node_address=.*$|wsrep_node_address=${WSREP_NODE_ADDRESS}|" /etc/mysql/conf.d/cluster.cnf
fi

# Set wsrep_cluster_address so that this node connects to all nodes that are
# already setup (the Kubernetes StatefulSet attempts to start the nodes in
# order). If this is the first node, it will bootstrap the cluster.
if [[ "$(hostname)" =~ ^(.*)-([0-9]+)$ ]]; then
  statefulset_name="${BASH_REMATCH[1]}"
  replica_index=${BASH_REMATCH[2]}
  domain="galera"
  cluster_address="gcomm://"

  for ((i=0; i < $replica_index; i++)); do
    cluster_address+="${statefulset_name}-${i}.${domain},"
  done

  sed -i -e "s|^wsrep_cluster_address=gcomm://|wsrep_cluster_address=${cluster_address}|" /etc/mysql/conf.d/cluster.cnf
else
  echo >&2 'error: expected hostname to be of the form "foo-N"'
  exit 1
fi

# Provide a random server ID for this replica.
sed -i -e "s/^server\-id=.*$/server-id=${RANDOM}/" /etc/mysql/my.cnf

# Finally, start MySQL, passing through any flags.
chown mysql:mysql "$INIT_SQL"
exec "$@" --datadir "$DATADIR" --ignore-db-dir "lost+found" --init_file "$INIT_SQL"

