# tiny-vless-ws

A zero-dependency, ultra-lightweight VLESS over WebSocket server written in Go. Ideal for running on resource-constrained environments (e.g. 0.1 CPU + 256MB RAM). Compiles to a static ~5.5MB Go binary and uses <5MB RAM.

Compatible with **v2rayNG / Xray Core**.

## Features

- **Standard Library Only**: No external Go dependencies (zero-dependency).
- **Supports TCP & UDP**: Proxies both TCP streams and DNS/UDP traffic.
- **Ultra-lightweight**: Multi-stage Dockerized alpine image uses minimal size, RAM, and CPU.

## Environment Variables

- `UUID` (Required): The authentication client UUID.
- `PORT` (Optional): Port to listen on (default: `8080`).
- `WS_PATH` (Optional): WebSocket path to serve (default: `/assets/js/main.js`).

## Docker

You can use the published GHCR container image:

```bash
docker run -d \
  --name tiny-vless-ws \
  -p 8080:8080 \
  -e UUID="YOUR-UUID-HERE" \
  -e WS_PATH="/assets/js/main.js" \
  ghcr.io/ntun7729/tiny-vless-ws:latest
```

## Client Configuration (v2rayNG / Xray)

Use standard VLESS+WS settings:
- **Protocol**: `VLESS`
- **Address**: `your-server.com`
- **Port**: `80 / 443`
- **UUID**: `<Your UUID>`
- **Transport**: `ws` (WebSocket)
- **Path**: `/assets/js/main.js`
- **TLS**: Specify if setup with reverse-proxy (e.g. Nginx or Cloudflare Tunnel)
