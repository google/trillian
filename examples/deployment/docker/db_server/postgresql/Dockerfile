FROM postgres:18.0@sha256:1ffc019dae94eca6b09a49ca67d37398951346de3c3d0cfe23d8d4ca33da83fb

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
