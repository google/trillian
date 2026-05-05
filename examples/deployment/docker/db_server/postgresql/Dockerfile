FROM postgres:18.3@sha256:78481659c47e862334611ccdaf7c369c986b3046da9857112f3b309114a65fb4

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
