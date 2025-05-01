FROM postgres:17.4@sha256:304ab813518754228f9f792f79d6da36359b82d8ecf418096c636725f8c930ad

# expects the build context to be: $GOPATH/src/github.com/google/trillian
COPY storage/postgresql/schema/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d
