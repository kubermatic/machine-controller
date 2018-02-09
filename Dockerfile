FROM alpine:3.7

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

COPY machine-controller /usr/local/bin

USER nobody
