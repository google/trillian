FROM postgres:18.3@sha256:69e8582b781cb44fa4557b98ed586fe68361e320d9b12f9707494335634f4f3d

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
