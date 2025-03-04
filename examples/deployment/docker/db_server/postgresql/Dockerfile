FROM postgres:17.4@sha256:81f32a88ec561664634637dd446487efd5f9d90996304b96210078e90e5c8b21

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
