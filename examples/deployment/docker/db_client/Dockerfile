FROM golang:1.21.1-bookworm@sha256:114b9cc39f435cfc34e2d48d0cb12cba2015a87f359f5dbf3ee3b6cf3591b7fb

RUN apt-get update && \
    apt-get install -y mariadb-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

# This is used to wait for new MySQL deployments to become ready e.g.
#  ./wait-for-it.sh localhost:3306 -- mysql
RUN ./examples/deployment/scripts/download-wait-for-it.sh

CMD [ 'mysql' ]
