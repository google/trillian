FROM golang:1.22.3-bookworm@sha256:c2bc4ef4064059a5dbd083e573dbd5d074858130fa3f1ed14807903b71ed99f1

RUN apt-get update && \
    apt-get install -y mariadb-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

# This is used to wait for new MySQL deployments to become ready e.g.
#  ./wait-for-it.sh localhost:3306 -- mysql
RUN ./examples/deployment/scripts/download-wait-for-it.sh

CMD [ 'mysql' ]
