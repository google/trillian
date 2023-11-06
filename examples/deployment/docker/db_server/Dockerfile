FROM gcr.io/trillian-opensource-ci/mysql8:8.0@sha256:89d1c5d48cee111cdc6348704044a60578f5c0af9dfc55b6e58e32a01906bd21

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
