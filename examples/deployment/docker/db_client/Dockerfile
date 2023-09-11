FROM golang:1.20.8-bookworm@sha256:b099af7250d673d249597b79049e3e53487c435acf285e07a36d4b49f9f4750a

RUN apt-get update && \
    apt-get install -y mariadb-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

# This is used to wait for new MySQL deployments to become ready e.g.
#  ./wait-for-it.sh localhost:3306 -- mysql
RUN ./examples/deployment/scripts/download-wait-for-it.sh

CMD [ 'mysql' ]
