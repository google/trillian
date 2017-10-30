FROM golang:1.9

ENV DB_USER=test \
    DB_PASSWORD=zaphod \
    DB_DATABASE=test \
    DB_HOST=127.0.0.0:3306

ENV HOST=0.0.0.0 \
    RPC_PORT=8090 \
    HTTP_PORT=8091

ENV DUMP_METRICS 0s

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get -v ./server/trillian_log_server

ENTRYPOINT /go/bin/trillian_log_server \
	--mysql_uri="${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST})/${DB_DATABASE}" \
	--rpc_endpoint="$HOST:$RPC_PORT" \
	--http_endpoint="$HOST:$HTTP_PORT" \
	--alsologtostderr

EXPOSE $RPC_PORT
EXPOSE $HTTP_PORT

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost:$HTTP_PORT/debug/vars || exit 1
