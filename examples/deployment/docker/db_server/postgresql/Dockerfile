FROM postgres:17.2@sha256:888402a8cd6075c5dc83a31f58287f13306c318eaad016661ed12e076f3e6341

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
