FROM postgres:18.1@sha256:3bb77a07b9ce8b8de0fe6d9b063adf20b9cca1068a1bcdc054cac71a69ce0ca6

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
