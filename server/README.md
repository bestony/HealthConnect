# HealthConnect server

## MCP

`healthd` serves a read-only MCP endpoint at `/mcp`.

The client sends the bootstrap token in the `Authorization` header. The token is in `data/bootstrap.token`. The file mode is `0600`. Do not commit the file. Do not log the token.

The token is not a tool argument. The endpoint reads only the user for that token.

```json
{
  "mcpServers": {
    "health-connect": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer hc_…"
      }
    }
  }
}
```

The iOS app copies this configuration. The app uses the server address that the user enters and appends `/mcp`. The verification code is the bearer token.

Claude Code can add the same endpoint with this command:

```text
claude mcp add --transport http health-connect 'http://127.0.0.1:8080/mcp' --header 'Authorization: Bearer hc_…'
```

The app copies this command with the same address and the same token.

Set `MCP_ENABLED=false` to disable `/mcp`. The default is `true`.

The default listen address is `127.0.0.1:8080`. If you change `HTTP_ADDR`, the token is the access control.

## Deploy

The container image listens on `0.0.0.0:8080`.
The Compose file does not publish that port.
Caddy obtains a certificate for `DOMAIN` and forwards traffic to `healthd`.

The iOS app accepts `https` for a public host.
Set the app server address to `https://<DOMAIN>`.
The MCP address is `https://<DOMAIN>/mcp`.

DuckDB accepts one writer process.
Run one `healthd` container for the data volume.

Build the image on the target machine architecture.
DuckDB provides static libraries for `linux/amd64` and `linux/arm64`.
The link step needs several gigabytes of memory.

### Requirements

- Docker Engine 24 or newer, with the Compose plugin.
- A DNS name that points at this host.
- Open TCP ports 80 and 443, and UDP port 443.
- Free disk space for the health data. A full export can use many gigabytes.

### Start

Copy the environment file:

```sh
cd server
cp .env.example .env
```

Set `DOMAIN` and `ACME_EMAIL` in `.env`.
Do not commit `.env`.

Start the stack:

```sh
docker compose up -d --build
```

Read the bootstrap token:

```sh
docker compose exec healthd cat /var/lib/healthconnect/bootstrap.token
```

Use this token as the verification code in the iOS app.
Use the same token as the bearer token for `/mcp`.
The file mode is `0600`. The logs do not contain the token.

### Configuration

| Variable | Compose value | Meaning |
| --- | --- | --- |
| `HTTP_ADDR` | `0.0.0.0:8080` | Listen address inside the container. Keep this value. |
| `DATA_DIR` | `/var/lib/healthconnect` | Data directory. The named volume `healthconnect-data` uses this path. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `MCP_ENABLED` | `true` | `false` disables `/mcp`. |
| `MAX_UPLOAD_BYTES` | `8589934592` | Maximum HTTP body size. The default is 8 GiB. |
| `MAX_UNCOMPRESSED_BYTES` | `17179869184` | Maximum uncompressed import size. The default is 16 GiB. |
| `DOMAIN` | none | Public DNS name for Caddy. |
| `ACME_EMAIL` | none | Email address for certificate notices. |

### Backup

Stop `healthd` before you copy the data volume.
A copy of a live DuckDB file can be incomplete.

```sh
docker compose stop healthd
docker run --rm \
  -v healthconnect-data:/from:ro \
  -v "$PWD":/to \
  alpine:3.22 \
  tar -C /from -cf /to/health-data.tar .
docker compose start healthd
```

Keep `health-data.tar` in private storage.
The archive contains the bootstrap token and the health data.

### Local image check

This check does not obtain a certificate.

```sh
docker build -t healthconnect-healthd:local .
docker run --rm -p 127.0.0.1:8080:8080 healthconnect-healthd:local
```

Open `http://127.0.0.1:8080/v1/healthz`.
The response is HTTP 200 and `{"status":"ok"}`.

