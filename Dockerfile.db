FROM mysql

ENV MYSQL_PASSWORD=zaphod \
    MYSQL_USER=test \
    MYSQL_DATABASE=test \
    MYSQL_RANDOM_ROOT_PASSWORD=yes

ADD storage/mysql/storage.sql /docker-entrypoint-initdb.d/storage.sql
