FROM gcr.io/trillian-opensource-ci/mysql5:5.7@sha256:edf7defdd589073b6659ae07e84be82af8362f30ffa4091c5ed422d6a55c50c1

# Patch the OS-level packages and remove unneeded dependencies.
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install -y procps \
    && apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" upgrade \
    && apt-get -y autoremove \
    && rm -rf /var/lib/apt/lists/*

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY examples/deployment/docker/db_server/mysql.cnf /etc/mysql/conf.d/trillian.cnf
COPY storage/mysql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d && \
    chmod 644 /etc/mysql/conf.d/trillian.cnf
