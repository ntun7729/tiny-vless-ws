# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w -buildid=" \
    -o /out/tiny-vless .

FROM scratch

COPY --from=builder /out/tiny-vless /tiny-vless

# The service binds to an unprivileged port and does not need root.
USER 65532:65532

ENV PORT=8080 \
    WS_PATH="/assets/js/main.js"

EXPOSE 8080

ENTRYPOINT ["/tiny-vless"]
