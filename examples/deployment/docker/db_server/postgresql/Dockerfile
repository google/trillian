FROM postgres:17.4@sha256:0ab5f0d74775b687e8cc67c3d55a7c8e2157c40a29171a48aefe6ce1cb3a2c3a

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
