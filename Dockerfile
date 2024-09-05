# Build release image
FROM alpine:latest

RUN apk add --no-cache bash ca-certificates

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "1000" \
    agent

COPY os-image-updater /usr/bin/os-image-updater

USER agent:agent
CMD ["/usr/bin/os-image-updater"]
