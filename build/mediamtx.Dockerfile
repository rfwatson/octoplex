FROM bluenviron/mediamtx:latest AS mediamtx

FROM alpine:3.21

RUN apk add --no-cache \
    bash \
    curl

COPY --from=mediamtx /mediamtx /usr/bin/mediamtx

CMD ["/usr/bin/mediamtx"]
