FROM gcr.io/trillian-opensource-ci/mysql5:5.7@sha256:c079e4e1cfa0e244f859fe37dc28d9c133835bc34098196132c461db91dd85f3

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
