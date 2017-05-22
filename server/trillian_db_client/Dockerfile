FROM golang:1.8

RUN apt-get update
RUN apt-get install -y mysql-client

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get -v ./server/trillian_log_server

ENV DB_USER=test \
    DB_PASSWORD=zaphod \
    DB_DATABASE=test

CMD [ 'mysql' ]
