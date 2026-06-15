# Build release image
FROM alpine:latest@sha256:a2d49ea686c2adfe3c992e47dc3b5e7fa6e6b5055609400dc2acaeb241c829f4

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
