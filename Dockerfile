FROM alpine:3.7

RUN apk add --no-cache ca-certificates cdrkit

COPY machine-controller /usr/local/bin
COPY machine-controller-userdata-* /usr/local/bin
COPY webhook /usr/local/bin

USER nobody
