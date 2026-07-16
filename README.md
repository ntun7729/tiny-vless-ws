# tiny-vless-ws

A small, zero-external-dependency VLESS-over-WebSocket server written in Go. It supports TCP forwarding and VLESS UDP packet framing, and is designed for simple container deployments behind a TLS-terminating reverse proxy.

> This server does not provide TLS by itself. Use a trusted reverse proxy, load balancer, or tunnel when exposing it to the internet.

## Highlights

- Standard-library-only Go implementation
- VLESS TCP and UDP forwarding over binary WebSocket frames
- Strict WebSocket handshake and frame validation
- Bounded WebSocket message allocation to reduce denial-of-service risk
- Graceful shutdown and startup/configuration validation
- Tiny embedded landing page at `/`
- `/healthz` liveness endpoint
- Minimal scratch-based container running as a non-root user
- Unit tests, race detection, vetting, formatting checks, and container CI

## Configuration

| Variable | Required | Default | Description |
|---|---:|---|---|
| `UUID` | Yes | — | VLESS client UUID. The all-zero UUID is rejected. |
| `PORT` | No | `8080` | Listening port, from `1` to `65535`. |
| `WS_PATH` | No | `/assets/js/main.js` | Exact WebSocket endpoint path. Normal HTTP requests to the default path receive the landing-page JavaScript. |
| `MAX_WS_MESSAGE_BYTES` | No | `4194304` | Maximum accepted WebSocket frame/message size. Allowed range: 1 KiB to 64 MiB. |

## Run with Docker

```bash
docker run --rm \
  --name tiny-vless-ws \
  -p 8080:8080 \
  -e UUID="00112233-4455-6677-8899-aabbccddeeff" \
  -e WS_PATH="/assets/js/main.js" \
  ghcr.io/ntun7729/tiny-vless-ws:latest
```

Check liveness:

```bash
curl --fail http://127.0.0.1:8080/healthz
```

## Web endpoints

- `/` serves a tiny status page.
- `/assets/js/main.js` serves its JavaScript during a normal `GET` or `HEAD` request.
- A WebSocket upgrade on the configured `WS_PATH` is routed to VLESS instead of static content.
- `/healthz` returns `ok` for liveness checks.

## Client settings

Use standard VLESS-over-WebSocket settings:

- Protocol: `VLESS`
- Address: your public hostname
- Port: normally `443` when TLS is terminated upstream
- UUID: the same value supplied to the server
- Transport: `ws`
- WebSocket path: the configured `WS_PATH`
- TLS: enabled at the reverse proxy or tunnel

The proxy must preserve WebSocket upgrade headers and route the configured path to this service.

### Nginx routing example

```nginx
location = / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
}

location = /healthz {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
}

location = /assets/js/main.js {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
}
```

## Build and test

The module has no third-party Go dependencies.

```bash
gofmt -w .
go vet ./...
go test -race ./...
go build -trimpath -ldflags="-s -w" -o tiny-vless-ws .
```

Build the container locally:

```bash
docker build -t tiny-vless-ws:local .
```

## Operational notes

- The WebSocket path is matched exactly.
- Client WebSocket frames must be masked, as required by RFC 6455.
- Oversized or malformed frames are rejected before large allocations occur.
- Long-lived proxied connections are intentionally allowed; place connection and rate limits at the edge when operating a public service.
- Do not reuse the example UUID.
