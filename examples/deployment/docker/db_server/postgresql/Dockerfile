FROM postgres:17.2@sha256:3267c505060a0052e5aa6e5175a7b41ab6b04da2f8c4540fc6e98a37210aa2d3

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
