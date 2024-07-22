FROM alpine:3.20@sha256:a59bbcb5abd79dd098c5f2de7f603aa7a90edf2d04f7c4a6fecc4eac08836290

RUN apk add --no-cache gettext

ADD envsubst_inplace.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
