FROM postgres:17.5@sha256:4d89c904835259bc58876520e56267ca07a4ebd6a027f7814bbbf91b50d685be

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
