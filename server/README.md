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

Set `MCP_ENABLED=false` to disable `/mcp`. The default is `true`.

The default listen address is `127.0.0.1:8080`. If you change `HTTP_ADDR`, the token is the access control.
