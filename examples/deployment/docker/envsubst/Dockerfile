FROM alpine:3.22@sha256:8a1f59ffb675680d47db6337b49d22281a139e9d709335b492be023728e11715

RUN apk add --no-cache gettext

ADD envsubst_inplace.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
