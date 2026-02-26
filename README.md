# Things Cloud MCP

An MCP server that connects AI assistants to Things 3 via Things Cloud.

## Features

- Streamable HTTP transport with OAuth 2.1 and Basic authentication
- 14 tools for listing, creating, editing, completing, trashing, and moving tasks, projects, areas, and tags
- Real-time sync with Things 3 apps on Mac, iPhone, and iPad
- Built with Go using [things-cloud-sdk](https://github.com/arthursoares/things-cloud-sdk) and [mcp-go](https://github.com/mark3labs/mcp-go)

## Quick Start

```
go build -o things-mcp .
THINGS_USERNAME=you@example.com THINGS_PASSWORD=secret ./things-mcp
```

The server listens on port 8080 by default (set `PORT` to override). The MCP endpoint is at `POST /mcp`.

## Live Demo

https://things-mcp.wenbo.io
