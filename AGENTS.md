# AGENTS.md

Things Cloud MCP: a remote, multi-user MCP server for Things 3, running as one Cloudflare Worker at https://thingscloudmcp.com. Source of truth is `apps/things-mcp` in the owner's infra monorepo; github.com/wbopan/things-cloud-mcp is its public mirror (copy changes over, then handle issues and PRs there).

## Build, test, deploy

```bash
npm install
npm run check    # tsc + node tests against a fake Things Cloud
npm run dev      # wrangler dev; needs .dev.vars with JWT_SECRET and CREDENTIALS_KEY
npm run deploy   # applies D1 migrations, then wrangler deploy
```

Deploying needs `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` in the environment (where the owner keeps them is in the infra repo's notes).

## Layout

- `src/index.ts`: Worker router, OAuth 2.1 server (D1), stateless MCP endpoint (JSON-RPC over POST /mcp, JSON responses), the `Account` Durable Object and its SQLite store.
- `src/session.ts`: one account's sync / validate / write lifecycle.
- `src/things.ts`: Things Cloud client, wire decoding, the in-memory task graph (`State`), date and recurrence helpers.
- `src/tools.ts`: the 23 tool handlers; `src/tools.json`: their definitions as served by tools/list (names, descriptions, input and output schemas, annotations).
- `src/diagnose.ts`: `things_diagnose`.
- `src/pages.ts`: the OAuth sign-in and success pages. `public/`: landing, docs and how-it-works pages, served as static assets before the Worker runs.
- `migrations/`: D1 schema.

## State

- D1 `things-mcp`: `clients` (dynamic registration), `auth_codes`, `refresh_tokens` (stored as `sha256:<hex>`), `credentials` (email to AES-GCM encrypted Things password), `diagnoses` (shared reports, 7 days).
- One `Account` Durable Object per Things account, keyed by the lowercased email. Its SQLite holds the history cursor plus the credential digest (`meta`) and every task, area, tag and checklist item as JSON (`ent`). After an eviction the graph is reloaded from SQLite and only the delta since the cursor is fetched.
- Secrets: `JWT_SECRET` (HS256 key, raw string bytes) and `CREDENTIALS_KEY` (base64url 32-byte AES key). Encrypted passwords use prefix `enc:v1:`, AAD `things-cloud-mcp/oauth-password/v1`, payload nonce||ciphertext. Rotating either secret signs everyone out.

## Safety invariants

Every operation for one account runs alone (`Session.serialize`): history cursors and the task graph advance atomically. Do not break this up.

Synchronization is fail-closed:

- Use the history key returned by Verify; never pick a history by size.
- A credential different from the last verified one is verified against Things Cloud before the cached graph is read.
- Initial load builds the whole graph first; incremental sync fetches into a cloned cursor and applies the validated delta atomically.
- Never fall back to a full rebuild after an incremental error. The one exception is a replaced history (Things Cloud account reset): a 404 or a regressed cursor, confirmed by Verify returning a different history key or a head below our cursor.
- Reject unknown schema versions, entity kinds, actions, malformed payloads, cursor regression and no-progress pagination. `Settings<digits>` records and the Mail to Things queue (`Command`, `Command3`, exact versions only) are ignored on purpose.

Writes:

- Validate envelopes, UUIDs, relationships, dates, recurrence and destructive confirmations before POST.
- Active lists inherit container state: a pending child of a completed, canceled or trashed heading/project is not active. Creating or moving into an inactive destination is rejected before POST.
- A transport failure or malformed response after POST is an uncertain commit: reconcile by reading history, never retry.
- 409 OutdatedAncestor is a clean rejection: sync again and resend, up to 3 times.
- 429 AbusePrevention on a GET backs off and retries up to 3 times; commits never retry on 429.
- After a confirmed commit, apply the exact events locally.
- Recurring creation is one commit with the template and its visible instance; templates stay hidden from read tools.
- Permanent area, tag and checklist deletion needs `confirm=true`; stopping recurrence needs `confirm_destructive=true`.

Never log credentials, bearer tokens, keys or Things response bodies.

## Tools

23 tools. A change to a tool updates `src/tools.json`, the handler in `src/tools.ts`, `public/docs.html` and `public/index.html` together. Every tool returns `structuredContent` as `{ "data": ... }` plus a JSON text copy; errors are `isError` results with a message.

## Testing against production

Do not use the deployed MCP tools to test uncommitted code; use `npm run check` or `wrangler dev`. Production checks are read-only unless a write to a disposable account is explicitly authorized.
