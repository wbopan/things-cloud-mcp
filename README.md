# Things Cloud MCP

An MCP server that connects AI assistants to Things 3 via Things Cloud.

## Features

- Streamable HTTP transport with OAuth 2.1 and Basic authentication
- Multi-user support — each user authenticates with their own Things Cloud credentials
- 14 tools for managing tasks, projects, areas, and tags
- Real-time sync with Things 3 apps on Mac, iPhone, and iPad
- Built with Go using [things-cloud-sdk](https://github.com/arthursoares/things-cloud-sdk) and [mcp-go](https://github.com/mark3labs/mcp-go)

## Quick Start

```
go build -o things-mcp .
./things-mcp
```

The server listens on port 8080 by default (set `PORT` to override). Optionally set `JWT_SECRET` for stable tokens across restarts.

- MCP endpoint: `POST /mcp`
- OAuth clients (Claude.ai, ChatGPT) authenticate via the built-in OAuth 2.1 flow
- CLI clients (Claude Code, Cursor, Windsurf) use Basic auth headers

## Live Demo

https://thingscloudmcp.com
