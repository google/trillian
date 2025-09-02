FROM postgres:17.6@sha256:29e0bb09c8e7e7fc265ea9f4367de9622e55bae6b0b97e7cce740c2d63c2ebc0

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
