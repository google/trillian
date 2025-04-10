FROM postgres:17.4@sha256:fe3f571d128e8efadcd8b2fde0e2b73ebab6dbec33f6bfe69d98c682c7d8f7bd

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
