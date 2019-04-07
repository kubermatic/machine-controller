FROM alpine:3.7

RUN apk add --no-cache ca-certificates cdrkit

COPY machine-controller machine-controller-userdata-* webhook /usr/local/bin/

USER nobody
