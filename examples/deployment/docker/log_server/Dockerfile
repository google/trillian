FROM golang:1.9

ENV DB_FLAG="--mysql_url=test:zaphod@tcp(127.0.0.1:3306)/test" \
    DB_PROVIDER="mysql"

ENV HOST=0.0.0.0 \
    RPC_PORT=8090 \
    HTTP_PORT=8091

ENV DUMP_METRICS 0s

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get ./server/trillian_log_server

ENTRYPOINT /go/bin/trillian_log_server \
  ${DB_FLAG} \
  --storage_system=${DB_PROVIDER} \
  --rpc_endpoint="$HOST:$RPC_PORT" \
  --http_endpoint="$HOST:$HTTP_PORT" \
  --alsologtostderr

EXPOSE $RPC_PORT
EXPOSE $HTTP_PORT

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost:$HTTP_PORT/debug/vars || exit 1
