FROM golang:1.21.1-bookworm@sha256:9c7ea4a4924ae24913401af45c9b6f439d0f782267584738d1cc1099d8b7a02c

RUN apt-get update && \
    apt-get install -y mariadb-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

# This is used to wait for new MySQL deployments to become ready e.g.
#  ./wait-for-it.sh localhost:3306 -- mysql
RUN ./examples/deployment/scripts/download-wait-for-it.sh

CMD [ 'mysql' ]
