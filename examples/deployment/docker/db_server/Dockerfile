FROM mysql:5.7

# expects the build context to be: $GOPATH/src/github.com/google/trillian 
COPY storage/mysql/storage.sql /docker-entrypoint-initdb.d/storage.sql
RUN chmod -R 775 /docker-entrypoint-initdb.d

