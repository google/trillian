FROM postgres:17.2@sha256:87ec5e0a167dc7d4831729f9e1d2ee7b8597dcc49ccd9e43cc5f89e808d2adae

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
