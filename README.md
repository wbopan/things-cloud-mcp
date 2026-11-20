# Things Cloud MCP

An MCP server that connects AI assistants to Things 3 via Things Cloud.

**Public endpoint: https://thingscloudmcp.com**

Add this URL to your MCP client and start managing Things 3 tasks with AI. Multi-user: each user signs in with their own Things Cloud account.

## Features

- Streamable HTTP transport with OAuth 2.1 (dynamic registration, client ID metadata documents, PKCE) and Basic authentication
- 23 tools for reading and managing tasks, projects, headings, areas, tags and checklists
- Real-time sync with Things 3 on Mac, iPhone and iPad
- Output schemas and structured content for every tool
- Fail-closed sync, uncertain-write reconciliation without automatic retry, and explicit confirmation for permanent deletion

Clients such as Claude.ai and ChatGPT use the OAuth flow; CLI clients (Claude Code, Cursor, Windsurf) can also send a Basic `Authorization` header.

## How it runs

The server is a single Cloudflare Worker written in TypeScript. OAuth state lives in D1, and every Things account gets its own Durable Object that keeps the account's task graph in SQLite and serializes its reads and writes. Things passwords are stored encrypted (AES-GCM) and refresh tokens only as hashes.

## Self-hosting

```bash
npm install
npx wrangler d1 create things-mcp      # put the id into wrangler.jsonc
npx wrangler secret put JWT_SECRET      # any long random string
npx wrangler secret put CREDENTIALS_KEY # 32 random bytes, base64url
npm run deploy
```

Change or remove `routes` in `wrangler.jsonc` to use your own domain. For local development put the two secrets in `.dev.vars` and run `npm run dev`; `npm run check` runs the type check and tests.

This project writes through a reverse-engineered, unofficial Things Cloud protocol. Keep a restorable Things backup and use a disposable account when testing writes.

Protocol knowledge comes from [things-cloud-sdk](https://github.com/arthursoares/things-cloud-sdk).
