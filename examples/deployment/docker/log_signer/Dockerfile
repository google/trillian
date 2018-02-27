FROM golang:1.9

ENV DB_FLAG="--mysql_url=test:zaphod@tcp(127.0.0.1:3306)/test" \
    DB_PROVIDER="mysql"

ENV HOST=0.0.0.0 \
    HTTP_PORT=8091

ENV SEQUENCER_GUARD_WINDOW=0s \
    FORCE_MASTER=true \
    SEQUENCER_INTERVAL=300ms \
    NUM_SEQ_FLAG=10 \
    BATCH_SIZE=2000


ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get ./server/trillian_log_signer

# Run the outyet command by default when the container starts.
ENTRYPOINT /go/bin/trillian_log_signer \
  ${DB_FLAG} \
  --storage_system=${DB_PROVIDER} \
  --http_endpoint="$HOST:$HTTP_PORT" \
  --sequencer_guard_window="$SEQUENCER_GUARD_WINDOW" \
  --sequencer_interval="$SEQUENCER_INTERVAL" \
  --num_sequencers="$NUM_SEQ_FLAG" \
  --batch_size="$BATCH_SIZE" \
  --force_master="$FORCE_MASTER" \
  --alsologtostderr

EXPOSE $HTTP_PORT

HEALTHCHECK --interval=5m --timeout=3s \
  CMD curl -f http://localhost:$HTTP_PORT/debug/vars || exit 1
