FROM golang:1.9

RUN apt-get update && \
    apt-get install -y mysql-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ENV DB_USER=test \
    DB_PASSWORD=zaphod \
    DB_DATABASE=test

# This is used to wait for new MySQL deployments to become ready e.g.
#  ./wait-for-it.sh localhost:3306 -- mysql
RUN ./examples/deployment/scripts/download-wait-for-it.sh

CMD [ 'mysql' ]
