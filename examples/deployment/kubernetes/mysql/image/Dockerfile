# Original work: Copyright 2016 The Kubernetes Authors.
# Modified work: Copyright 2017 Google LLC. All Rights Reserved.
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

FROM ubuntu:jammy

# add our user and group first to make sure their IDs get assigned
# consistently, regardless of whatever dependencies get added
RUN groupadd -r mysql && useradd -r -g mysql mysql

ENV MYSQL_VERSION 5.7
ENV TERM linux

# Install perl.
# Install dig, for discovering IP addresses of other Galera nodes.
RUN DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y dnsutils perl --no-install-recommends
RUN rm -rf /var/lib/apt/lists/*

RUN apt-key adv --keyserver keys.gnupg.net --recv-keys 1C4CBDCDCD2EFD2A 9334A25F8507EFA5

RUN echo "deb http://repo.percona.com/apt jammy main" > /etc/apt/sources.list.d/percona.list
RUN echo "deb-src http://repo.percona.com/apt jammy main" >> /etc/apt/sources.list.d/percona.list

RUN { \
                echo percona-server-server-${MYSQL_VERSION} percona-server-server/root_password password ''; \
        } | debconf-set-selections \
        && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y percona-xtradb-cluster-client-"${MYSQL_VERSION}" \
           percona-xtradb-cluster-common-"${MYSQL_VERSION}" percona-xtradb-cluster-server-"${MYSQL_VERSION}" \
        && rm -rf /var/lib/apt/lists/*

COPY my.cnf /etc/mysql/my.cnf
COPY cluster.cnf /etc/mysql/conf.d/cluster.cnf
RUN mkdir -p /var/lib/mysql
RUN chown -R mysql:mysql /etc/mysql /var/lib/mysql
VOLUME /var/lib/mysql

COPY docker-entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 3306 4444 4567 4568
CMD ["mysqld"]
