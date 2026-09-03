# Things Cloud MCP

An MCP server that connects AI assistants to Things 3 via Things Cloud.

**Public endpoint: https://thingscloudmcp.com**

Add this URL to your MCP client and start managing Things 3 tasks with AI. Multi-user — each user authenticates with their own Things Cloud credentials.

## Features

- Streamable HTTP transport with OAuth 2.1 and Basic authentication
- Multi-user support with per-user operation serialization and credential-bound sessions
- 23 tools for reading and managing tasks, projects, headings, areas, tags, and checklists
- Real-time sync with Things 3 apps on Mac, iPhone, and iPad
- MCP output schemas and structured content for every tool
- Fail-closed cursor/schema validation, uncertain-write reconciliation without automatic retry, and explicit confirmation for permanent deletion

## Self-hosting

If you prefer to host your own instance:

```bash
go build -o things-mcp .
./things-mcp
```

The server listens on port 8080 by default (set `PORT` to override). Optionally set `JWT_SECRET` for stable tokens across restarts. Things passwords stored for OAuth are encrypted with AES-GCM; set a durable high-entropy `CREDENTIALS_SECRET`, or persist the generated `DATA_DIR/credentials.key` alongside `oauth.db`. Refresh tokens are stored as hashes.

**Timezone**: set `MCP_TIMEZONE` to your IANA zone (e.g. `Europe/Berlin`, `America/New_York`) so that "today", "tonight", and the Today-filter logic match your calendar instead of the server's UTC clock. Falls back to `TZ`, then UTC. Without this, users in non-UTC zones can see tasks land on the wrong day around midnight.

- OAuth clients (Claude.ai, ChatGPT) authenticate via the built-in OAuth 2.1 flow
- CLI clients (Claude Code, Cursor, Windsurf) use Basic auth headers

### Deploy to Fly.io

The included `Dockerfile` stores OAuth state and generated keys under `/data`. To deploy on [Fly.io](https://fly.io), create a `fly.toml` for the app and mount the volume at `/data`:

```bash
brew install flyctl
fly auth login

fly launch --no-deploy                   # generate and review fly.toml
fly volumes create data --size 1         # 1 GB persistent volume for tokens and JWT secret
fly secrets set JWT_SECRET=$(openssl rand -hex 32)
fly secrets set CREDENTIALS_SECRET=$(openssl rand -hex 32)
fly deploy
```

**Important:** The persistent volume is required. Without it, OAuth state and any generated credential-encryption key are stored on ephemeral disk. Losing the encryption key makes persisted credentials intentionally unreadable. Back up `oauth.db` and the matching key together before deployment or migration.

This project writes through a reverse-engineered, unofficial Things Cloud protocol. Keep a restorable Things for Mac backup and use a disposable account for write validation. Production health checks should remain read-only unless a real write is explicitly intended.

Your endpoint will normally be `https://<app-name>.fly.dev`; verify current Fly configuration and pricing before deployment.

Built with [things-cloud-sdk](https://github.com/arthursoares/things-cloud-sdk) and [mcp-go](https://github.com/mark3labs/mcp-go).
