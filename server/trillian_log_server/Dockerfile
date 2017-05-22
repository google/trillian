FROM golang:1.8

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get -v ./server/trillian_log_server

ENV DB_USER=test \
    DB_PASSWORD=zaphod \
    DB_DATABASE=test \
    DB_HOST=127.0.0.0:3306

ENV RPC_HOST=localhost \
    RPC_PORT=8090 \
    HOST=0.0.0.0 \
    HTTP_PORT=8091

ENV DUMP_METRICS 0s

ENTRYPOINT /go/bin/trillian_log_server \
  --mysql_uri="${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST})/${DB_DATABASE}" \
  --rpc_endpoint="$RPC_HOST:$RPC_PORT" \
  --http_endpoint="$HTTP_HOST:$HTTP_PORT" \
  --dump_metrics_interval="$DUMP_METRICS" \
  --logtostderr

EXPOSE $RPC_PORT
EXPOSE $HTTP_PORT

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost:$HTTP_PORT/debug/vars || exit 1
