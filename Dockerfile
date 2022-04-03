FROM golang:1.18 AS build

WORKDIR /build

# Download and cache dependencies
COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go mod verify

# Copy application files
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s"


# Build release image
FROM alpine:latest

RUN apk add --no-cache ca-certificates

RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "1000" \
    "user"

COPY --from=build /build/os-image-updater /usr/bin/os-image-updater

USER user:user
CMD ["/usr/bin/os-image-updater"]
