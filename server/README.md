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

The container listens on `0.0.0.0:8080` and serves HTTP.
Terminate TLS in the platform in front of the container.
The iOS app accepts `https` for a public host.
Set the app server address to `https://<public host>`.
The MCP address is `https://<public host>/mcp`.

DuckDB accepts one writer process.
Run one container for the data directory.

Build the image on the target machine architecture.
DuckDB provides static libraries for `linux/amd64` and `linux/arm64`.
The link step needs several gigabytes of memory.

The process starts as root.
It gives `/var/lib/healthconnect` to the user `healthd`, and then it drops privileges.

### CapRover

The repository root contains `captain-definition`.
CapRover builds `server/Dockerfile`. The build context is the repository root.

1. Create an app.
2. Set the Container HTTP Port to `8080`.
3. Add a persistent directory. The container path is `/var/lib/healthconnect`.
4. Set the instance count to `1`.
5. Enable HTTPS for the app domain.
6. Enable WebSocket support so a long MCP stream stays open.
7. Deploy this repository.

Read the bootstrap token on the CapRover host.
Replace `APP` with the app name.

```sh
docker exec "$(docker ps -q -f name=srv-captain--APP)" cat /var/lib/healthconnect/bootstrap.token
```

Use this token as the verification code in the iOS app.
Use the same token as the bearer token for `/mcp`.
The file mode is `0600`. The logs do not contain the token.

The iOS app sends batch bodies of at most 8 MB.
Raise the CapRover upload limit when a request body is larger than that limit.

### Compose

Use Compose on a host whose proxy already terminates TLS.
The Compose file publishes port `8080`.

```sh
cd server
docker compose up -d --build
docker compose exec -u root healthd cat /var/lib/healthconnect/bootstrap.token
```

Copy `server/.env.example` to `server/.env` when you want to change a setting.
Do not commit `.env`.

### Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `HTTP_ADDR` | `0.0.0.0:8080` | Listen address inside the container. Keep this value. |
| `DATA_DIR` | `/var/lib/healthconnect` | Data directory. Persist this path. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error`. |
| `MCP_ENABLED` | `true` | `false` disables `/mcp`. |
| `MAX_UPLOAD_BYTES` | `8589934592` | Maximum HTTP body size. The default is 8 GiB. |
| `MAX_UNCOMPRESSED_BYTES` | `17179869184` | Maximum uncompressed import size. The default is 16 GiB. |

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

Run these commands from the repository root:

```sh
docker build -f server/Dockerfile -t healthconnect-healthd:local .
docker run --rm -p 127.0.0.1:8080:8080 healthconnect-healthd:local
```

Open `http://127.0.0.1:8080/v1/healthz`.
The response is HTTP 200 and `{"status":"ok"}`.

