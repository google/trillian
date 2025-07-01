FROM postgres:17.5@sha256:3962158596daaef3682838cc8eb0e719ad1ce520f88e34596ce8d5de1b6330a1

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
