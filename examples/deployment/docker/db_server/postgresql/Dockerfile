FROM postgres:17.1@sha256:163763c8afd28cae69035ce84b12d8180179559c747c0701b3cad17818a0dbc5

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
