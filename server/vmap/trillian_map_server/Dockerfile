FROM golang

ENV DB_USER=test \
    DB_PASSWORD=zaphod \
    DB_DATABASE=test \
    DB_HOST=127.0.0.0:3306 

ENV HOST=0.0.0.0 \
    RPC_PORT=8090 \
    HTTP_PORT=8091 

ENV SEQUENCER_GUARD_WINDOW=0s \
    DUMP_METRICS=0s

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian 

RUN go get -v ./server/vmap/trillian_map_server

# Run the outyet command by default when the container starts.
ENTRYPOINT /go/bin/trillian_map_server \
	--mysql_uri="${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST})/${DB_DATABASE}" \
	--rpc_endpoint="$HOST:$RPC_PORT" \
	--http_endpoint="$HOST:$HTTP_PORT" \ 
	--alsologtostderr \

EXPOSE $HTTP_PORT

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost:$HTTP_PORT/debug/vars || exit 1

