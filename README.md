# tiny-vless-ws

[![CI](https://github.com/ntun7729/tiny-vless-ws/actions/workflows/ci.yml/badge.svg)](https://github.com/ntun7729/tiny-vless-ws/actions/workflows/ci.yml)

A zero-dependency VLESS-over-WebSocket server written in Go for small, resource-constrained deployments.

Compatible with **v2rayNG** and **Xray Core**.

## Features

- Uses only the Go standard library.
- Proxies TCP streams and VLESS UDP packets.
- Supports masked and fragmented WebSocket frames.
- Builds as a static binary.
- Ships in a minimal `scratch` container and runs as a non-root user.
- Tests, vetting, native builds, and container builds run on pull requests.

## Configuration

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `UUID` | Yes | — | VLESS client UUID. Treat it as a secret. |
| `PORT` | No | `8080` | HTTP port used by the WebSocket server. |
| `WS_PATH` | No | `/assets/js/main.js` | WebSocket endpoint. A leading slash is added when omitted. |

## Run with Docker

```bash
docker run -d \
  --name tiny-vless-ws \
  --restart unless-stopped \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges:true \
  -p 8080:8080 \
  -e UUID="YOUR-UUID-HERE" \
  -e WS_PATH="/assets/js/main.js" \
  ghcr.io/ntun7729/tiny-vless-ws:latest
```

The container does not require a writable filesystem or Linux capabilities. It listens on an unprivileged port and runs as UID/GID `65532`.

## Build and test locally

Go 1.25 or newer is required.

```bash
go test ./...
go vet ./...
go build -trimpath -buildvcs=false -ldflags="-s -w" -o tiny-vless .
```

Build the container locally:

```bash
docker build -t tiny-vless-ws:local .
```

## Client configuration

Use standard VLESS + WebSocket settings:

| Setting | Value |
| --- | --- |
| Protocol | `VLESS` |
| Address | Your server hostname |
| Port | `80` or `443` when using a reverse proxy |
| UUID | The same value supplied through `UUID` |
| Transport | `ws` |
| Path | The configured `WS_PATH` |
| TLS | Enable when TLS is terminated by a reverse proxy or tunnel |

## Deployment notes

This server does not terminate TLS. Put it behind a maintained TLS reverse proxy or tunnel for internet-facing deployments. Keep the UUID private, expose only the intended listener, and update the container image regularly.
