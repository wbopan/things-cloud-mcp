# Things Cloud MCP

An MCP server that connects AI assistants to Things 3 via Things Cloud.

**Public endpoint: https://thingscloudmcp.com/mcp**

Add this URL to your MCP client and start managing Things 3 tasks with AI. Multi-user — each user authenticates with their own Things Cloud credentials.

## Features

- Streamable HTTP transport with OAuth 2.1 and Basic authentication
- Multi-user support with per-user credentials
- 14 tools for managing tasks, projects, areas, and tags
- Real-time sync with Things 3 apps on Mac, iPhone, and iPad

## Self-hosting

If you prefer to host your own instance:

```
go build -o things-mcp .
./things-mcp
```

The server listens on port 8080 by default (set `PORT` to override). Optionally set `JWT_SECRET` for stable tokens across restarts.

- OAuth clients (Claude.ai, ChatGPT) authenticate via the built-in OAuth 2.1 flow
- CLI clients (Claude Code, Cursor, Windsurf) use Basic auth headers

Built with [things-cloud-sdk](https://github.com/arthursoares/things-cloud-sdk) and [mcp-go](https://github.com/mark3labs/mcp-go).
