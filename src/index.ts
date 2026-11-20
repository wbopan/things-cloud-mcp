// Things Cloud MCP on Cloudflare Workers.
//
// The Worker serves the static site (public/), the OAuth 2.1 authorization
// server (state in D1) and the stateless MCP endpoint. Every Things account
// gets one Durable Object (keyed by its lowercased email) that keeps the
// account's history cursor and task graph in its SQLite storage and runs the
// whole sync / validate / write lifecycle one operation at a time.

import { DurableObject } from "cloudflare:workers";
import { API_ENDPOINT, ThingsClient, State, EntityKind } from "./things";
import { Session, Meta, Store } from "./session";
import { LOGIN_PAGE, SUCCESS_PAGE } from "./pages";
import TOOLS from "./tools.json";

interface Env {
  DB: D1Database;
  ACCOUNT: DurableObjectNamespace<Account>;
  JWT_SECRET: string;
  CREDENTIALS_KEY: string;
}

const SERVER_INFO = {
  name: "Things Cloud MCP",
  version: "2.0.0",
  icons: [{ src: "https://thingscloudmcp.com/favicon.svg", mimeType: "image/svg+xml" }],
};
const INSTRUCTIONS =
  "Things Cloud MCP server for managing Things 3 tasks, projects, areas, and tags. " +
  "Use things_find_tasks with filters (area, project, status, tag) to find tasks, and things_overview for a quick summary. " +
  "Use things_edit_item to modify a task or project's title, notes, dates, tags, or status " +
  "(status=completed to complete, status=canceled to cancel), and to move items between projects and areas. " +
  "Use things_create_task, things_create_project, things_create_area, or things_create_tag to create new items. " +
  "All changes sync to Things 3 apps (Mac, iPhone, iPad) in real-time via Things Cloud.";
const PROTOCOL_VERSIONS = ["2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"];
const TOOL_NAMES = new Set((TOOLS as any[]).map((t) => t.name));

const SCOPE = "things:manage";
const ACCESS_TTL = 3600;
const REFRESH_TTL = 30 * 86400;
const CODE_TTL = 600;
const DIAGNOSIS_TTL = 7 * 86400;

// ---------------------------------------------------------------------------
// Account Durable Object
// ---------------------------------------------------------------------------

export class Account extends DurableObject<Env> {
  private session: Session;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    const sql = ctx.storage.sql;
    sql.exec("CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL)");
    sql.exec("CREATE TABLE IF NOT EXISTS ent (kind TEXT NOT NULL, uuid TEXT NOT NULL, j TEXT NOT NULL, PRIMARY KEY (kind, uuid))");
    this.session = new Session(new SqlStore(ctx.storage), API_ENDPOINT);
  }

  callTool(email: string, password: string, name: string, args: Record<string, any>) {
    return this.session.callTool(email, password, name, args);
  }

  diagnose(email: string, password: string) {
    return this.session.runDiagnose(email, password);
  }
}

class SqlStore implements Store {
  constructor(private storage: DurableObjectStorage) {}

  loadMeta(): Meta | null {
    const row = this.storage.sql.exec<{ v: string }>("SELECT v FROM meta WHERE k = 'meta'").toArray()[0];
    return row ? JSON.parse(row.v) : null;
  }

  loadState(): State {
    const state = new State();
    for (const row of this.storage.sql.exec<{ kind: EntityKind; uuid: string; j: string }>("SELECT kind, uuid, j FROM ent ORDER BY rowid")) {
      state.mapFor(row.kind).set(row.uuid, JSON.parse(row.j));
    }
    return state;
  }

  save(meta: Meta, state: State, changed: { kind: EntityKind; uuid: string }[]): void {
    const sql = this.storage.sql;
    this.storage.transactionSync(() => {
      sql.exec("INSERT OR REPLACE INTO meta (k, v) VALUES ('meta', ?)", JSON.stringify(meta));
      for (const { kind, uuid } of changed) {
        const e = state.get(kind, uuid);
        if (e) sql.exec("INSERT OR REPLACE INTO ent (kind, uuid, j) VALUES (?, ?, ?)", kind, uuid, JSON.stringify(e));
        else sql.exec("DELETE FROM ent WHERE kind = ? AND uuid = ?", kind, uuid);
      }
    });
  }

  replaceAll(meta: Meta, state: State): void {
    const sql = this.storage.sql;
    this.storage.transactionSync(() => {
      sql.exec("DELETE FROM ent");
      sql.exec("INSERT OR REPLACE INTO meta (k, v) VALUES ('meta', ?)", JSON.stringify(meta));
      for (const kind of ["task", "area", "tag", "checklist"] as EntityKind[]) {
        for (const [uuid, e] of state.mapFor(kind)) sql.exec("INSERT INTO ent (kind, uuid, j) VALUES (?, ?, ?)", kind, uuid, JSON.stringify(e));
      }
    });
  }
}

function account(env: Env, email: string) {
  return env.ACCOUNT.getByName(email.trim().toLowerCase());
}

// ---------------------------------------------------------------------------
// Encoding and crypto helpers
// ---------------------------------------------------------------------------

const enc = new TextEncoder();

function b64url(bytes: Uint8Array): string {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function unb64url(s: string): Uint8Array {
  s = s.replace(/-/g, "+").replace(/_/g, "/");
  while (s.length % 4) s += "=";
  return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
}

function randomToken(n: number): string {
  return b64url(crypto.getRandomValues(new Uint8Array(n)));
}

async function sha256(s: string): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", enc.encode(s)));
}

const hex = (b: Uint8Array) => [...b].map((x) => x.toString(16).padStart(2, "0")).join("");

function timingSafeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let d = 0;
  for (let i = 0; i < a.length; i++) d |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return d === 0;
}

const rfc3339 = (sec: number) => new Date(sec * 1000).toISOString().replace(/\.\d{3}Z$/, "Z");
const nowSec = () => Math.floor(Date.now() / 1000);

// Refresh tokens are stored only as hashes.
async function refreshKey(token: string): Promise<string> {
  return "sha256:" + hex(await sha256(token));
}

// Things passwords at rest: AES-256-GCM, same format as the Go server so the
// migrated rows decrypt unchanged.
const CRED_PREFIX = "enc:v1:";
const CRED_AAD = enc.encode("things-cloud-mcp/oauth-password/v1");
let aesKey: Promise<CryptoKey> | null = null;

function credentialKey(env: Env): Promise<CryptoKey> {
  if (!aesKey) {
    const raw = unb64url((env.CREDENTIALS_KEY ?? "").trim());
    if (raw.length !== 32) throw new Error("CREDENTIALS_KEY must be 32 bytes, base64url encoded");
    aesKey = crypto.subtle.importKey("raw", raw, "AES-GCM", false, ["encrypt", "decrypt"]);
  }
  return aesKey;
}

async function encryptPassword(env: Env, password: string): Promise<string> {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const ct = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv, additionalData: CRED_AAD }, await credentialKey(env), enc.encode(password)));
  const out = new Uint8Array(iv.length + ct.length);
  out.set(iv);
  out.set(ct, iv.length);
  return CRED_PREFIX + b64url(out);
}

async function decryptPassword(env: Env, stored: string): Promise<string> {
  if (!stored.startsWith(CRED_PREFIX)) throw new Error("credential is not encrypted");
  const payload = unb64url(stored.slice(CRED_PREFIX.length));
  if (payload.length < 12) throw new Error("encrypted credential is truncated");
  const pt = await crypto.subtle.decrypt({ name: "AES-GCM", iv: payload.slice(0, 12), additionalData: CRED_AAD }, await credentialKey(env), payload.slice(12));
  return new TextDecoder().decode(pt);
}

let hmacKey: Promise<CryptoKey> | null = null;

function jwtKey(env: Env): Promise<CryptoKey> {
  if (!hmacKey) hmacKey = crypto.subtle.importKey("raw", enc.encode(env.JWT_SECRET), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  return hmacKey;
}

async function jwtSign(env: Env, input: string): Promise<string> {
  return b64url(new Uint8Array(await crypto.subtle.sign("HMAC", await jwtKey(env), enc.encode(input))));
}

async function createJWT(env: Env, claims: Record<string, any>): Promise<string> {
  const input = b64url(enc.encode('{"alg":"HS256","typ":"JWT"}')) + "." + b64url(enc.encode(JSON.stringify(claims)));
  return input + "." + (await jwtSign(env, input));
}

async function parseJWT(env: Env, token: string): Promise<Record<string, any> | null> {
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  if (!timingSafeEqual(parts[2], await jwtSign(env, parts[0] + "." + parts[1]))) return null;
  let claims: any;
  try {
    claims = JSON.parse(new TextDecoder().decode(unb64url(parts[1])));
  } catch {
    return null;
  }
  if (!claims || typeof claims !== "object") return null;
  if (typeof claims.exp === "number" && nowSec() > claims.exp) return null;
  return claims;
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

function json(status: number, body: any, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

const oauthError = (status: number, error: string, desc: string) => json(status, { error, error_description: desc });
const html = (body: string) => new Response(body, { headers: { "Content-Type": "text/html; charset=utf-8" } });
const methodNotAllowed = () => new Response("Method not allowed\n", { status: 405 });

function escapeHTML(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

async function formOf(req: Request): Promise<URLSearchParams> {
  try {
    return new URLSearchParams(await req.text());
  } catch {
    return new URLSearchParams();
  }
}

// ---------------------------------------------------------------------------
// OAuth clients (registered or Client ID Metadata Documents)
// ---------------------------------------------------------------------------

interface Client {
  client_id: string;
  client_name: string;
  redirect_uris: string[];
}

const cimdCache = new Map<string, { client: Client; expires: number }>();

function isCIMD(clientID: string): boolean {
  try {
    const u = new URL(clientID);
    return u.protocol === "https:" && !!u.host && u.pathname !== "" && u.pathname !== "/";
  } catch {
    return false;
  }
}

async function fetchCIMD(clientID: string): Promise<Client | null> {
  const hit = cimdCache.get(clientID);
  if (hit && Date.now() < hit.expires) return hit.client;
  try {
    const resp = await fetch(clientID, { headers: { Accept: "application/json" }, redirect: "manual", signal: AbortSignal.timeout(10_000) });
    if (resp.status !== 200) return null;
    const body = await resp.text();
    if (body.length > 64 << 10) return null;
    const doc = JSON.parse(body);
    if (doc?.client_id !== clientID || !Array.isArray(doc.redirect_uris) || doc.redirect_uris.length === 0) return null;
    const client: Client = { client_id: clientID, client_name: typeof doc.client_name === "string" && doc.client_name ? doc.client_name : "Unknown Client", redirect_uris: doc.redirect_uris.map(String) };
    cimdCache.set(clientID, { client, expires: Date.now() + 3600_000 });
    return client;
  } catch {
    return null;
  }
}

async function lookupClient(env: Env, clientID: string): Promise<Client | null> {
  if (!clientID) return null;
  const row = await env.DB.prepare("SELECT client_id, client_name, redirect_uris FROM clients WHERE client_id = ?").bind(clientID).first<any>();
  if (row) return { client_id: row.client_id, client_name: row.client_name, redirect_uris: JSON.parse(row.redirect_uris) ?? [] };
  return isCIMD(clientID) ? fetchCIMD(clientID) : null;
}

// Exact match, or a loopback URI registered without a port against any port
// (RFC 8252; Claude Code registers http://localhost/callback).
function redirectAllowed(client: Client, redirectURI: string): boolean {
  let bare = redirectURI;
  try {
    const u = new URL(redirectURI);
    if (u.protocol === "http:" && u.port && (u.hostname === "localhost" || u.hostname === "127.0.0.1")) bare = redirectURI.replace(":" + u.port, "");
  } catch {}
  return client.redirect_uris.some((uri) => uri === redirectURI || uri === bare);
}

// ---------------------------------------------------------------------------
// OAuth endpoints
// ---------------------------------------------------------------------------

function protectedResourceMetadata(base: string) {
  return json(200, { resource: base, authorization_servers: [base], scopes_supported: [SCOPE], bearer_methods_supported: ["header"] });
}

function authServerMetadata(base: string) {
  return json(200, {
    issuer: base,
    authorization_endpoint: base + "/authorize",
    token_endpoint: base + "/token",
    registration_endpoint: base + "/register",
    scopes_supported: [SCOPE],
    response_types_supported: ["code"],
    grant_types_supported: ["authorization_code", "refresh_token"],
    token_endpoint_auth_methods_supported: ["none", "client_secret_basic"],
    code_challenge_methods_supported: ["S256"],
    client_id_metadata_document_supported: true,
  });
}

async function register(req: Request, env: Env): Promise<Response> {
  let body: any;
  try {
    body = await req.json();
  } catch {
    return oauthError(400, "invalid_request", "invalid JSON body");
  }
  const list = (v: any) => (Array.isArray(v) ? v.filter((x) => typeof x === "string") : []);
  const redirectURIs = list(body?.redirect_uris);
  if (redirectURIs.length === 0) return oauthError(400, "invalid_request", "redirect_uris is required");
  const name = typeof body.client_name === "string" && body.client_name ? body.client_name : "Unknown Client";
  const grantTypes = list(body.grant_types).length ? list(body.grant_types) : ["authorization_code"];
  const responseTypes = list(body.response_types).length ? list(body.response_types) : ["code"];
  const clientID = randomToken(24);
  await env.DB.prepare("INSERT INTO clients (client_id, client_secret, client_name, redirect_uris, grant_types, response_types, created_at) VALUES (?, '', ?, ?, ?, ?, ?)")
    .bind(clientID, name, JSON.stringify(redirectURIs), JSON.stringify(grantTypes), JSON.stringify(responseTypes), rfc3339(nowSec()))
    .run();
  return json(201, { client_id: clientID, client_name: name, redirect_uris: redirectURIs, grant_types: grantTypes, response_types: responseTypes });
}

function loginPage(clientName: string, error: string, query: string): Response {
  const subtitle = clientName ? `<strong>${escapeHTML(clientName)}</strong> wants to access your tasks` : "Authorize access to your tasks";
  const err = error ? `<div class="auth-error">${escapeHTML(error)}</div>` : "";
  return html(LOGIN_PAGE.replace("{{subtitle}}", () => subtitle).replace("{{error}}", () => err).replace("{{query}}", () => escapeHTML(query)));
}

async function authorize(req: Request, env: Env, url: URL): Promise<Response> {
  const q = url.searchParams;
  const query = q.toString();
  const clientID = q.get("client_id") ?? "";
  const redirectURI = q.get("redirect_uri") ?? "";
  const state = q.get("state") ?? "";
  const challenge = q.get("code_challenge") ?? "";
  if (q.get("response_type") !== "code") return loginPage("", "Unsupported response_type. Must be 'code'.", query);
  if (!clientID || !redirectURI || !state) return loginPage("", "Missing required parameters: client_id, redirect_uri, state.", query);
  if (!challenge || q.get("code_challenge_method") !== "S256") return loginPage("", "PKCE required: code_challenge and code_challenge_method=S256.", query);
  const client = await lookupClient(env, clientID);
  if (!client) return loginPage("", "Unknown client_id.", query);
  if (!redirectAllowed(client, redirectURI)) return loginPage("", "Invalid redirect_uri.", query);
  if (req.method === "GET") return loginPage(client.client_name, "", query);

  const form = await formOf(req);
  const email = form.get("email") ?? "";
  const password = form.get("password") ?? "";
  if (!email || !password) return loginPage(client.client_name, "Email and password are required.", query);
  try {
    await new ThingsClient(email, password, API_ENDPOINT).verify();
  } catch {
    return loginPage(client.client_name, "Invalid Things Cloud credentials.", query);
  }
  const code = randomToken(32);
  await env.DB.prepare("INSERT INTO auth_codes (code, client_id, redirect_uri, email, password, code_challenge, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)")
    .bind(code, clientID, redirectURI, email, await encryptPassword(env, password), challenge, nowSec() + CODE_TTL)
    .run();
  const location = redirectURI + (redirectURI.includes("?") ? "&" : "?") + "code=" + encodeURIComponent(code) + "&state=" + encodeURIComponent(state);
  return html(SUCCESS_PAGE.replace('"{{redirect_url}}"', () => JSON.stringify(location).replace(/</g, "\\u003c")));
}

async function issueTokens(env: Env, base: string, email: string, refresh: string): Promise<Response> {
  const now = nowSec();
  const access = await createJWT(env, { exp: now + ACCESS_TTL, iat: now, iss: base, scope: SCOPE, sub: email });
  return json(200, { access_token: access, expires_in: ACCESS_TTL, refresh_token: refresh, scope: SCOPE, token_type: "Bearer" }, { "Cache-Control": "no-store" });
}

async function token(req: Request, env: Env, base: string): Promise<Response> {
  const form = await formOf(req);
  const grant = form.get("grant_type");
  const now = nowSec();
  if (grant === "authorization_code") {
    const code = form.get("code") ?? "";
    const clientID = form.get("client_id") ?? "";
    const redirectURI = form.get("redirect_uri") ?? "";
    const verifier = form.get("code_verifier") ?? "";
    if (!code || !clientID || !redirectURI || !verifier) return oauthError(400, "invalid_request", "missing required parameters: code, client_id, redirect_uri, code_verifier");
    const ac = await env.DB.prepare("SELECT * FROM auth_codes WHERE code = ?").bind(code).first<any>();
    if (!ac) return oauthError(400, "invalid_grant", "unknown authorization code");
    if (ac.used) return oauthError(400, "invalid_grant", "authorization code already used");
    if (now > ac.expires_at) return oauthError(400, "invalid_grant", "authorization code expired");
    if (ac.client_id !== clientID) return oauthError(400, "invalid_grant", "client_id mismatch");
    if (ac.redirect_uri !== redirectURI) return oauthError(400, "invalid_grant", "redirect_uri mismatch");
    if (!timingSafeEqual(b64url(await sha256(verifier)), ac.code_challenge)) return oauthError(400, "invalid_grant", "PKCE verification failed");
    const claimed = await env.DB.prepare("UPDATE auth_codes SET used = 1 WHERE code = ? AND used = 0").bind(code).run();
    if (!claimed.meta.changes) return oauthError(400, "invalid_grant", "authorization code already used");
    const refresh = randomToken(32);
    await env.DB.batch([
      env.DB.prepare("INSERT OR REPLACE INTO credentials (email, password) VALUES (?, ?)").bind(ac.email, ac.password),
      env.DB.prepare("INSERT INTO refresh_tokens (token, email, password, client_id, expires_at) VALUES (?, ?, ?, ?, ?)").bind(await refreshKey(refresh), ac.email, ac.password, clientID, rfc3339(now + REFRESH_TTL)),
      env.DB.prepare("DELETE FROM auth_codes WHERE expires_at < ?").bind(now - 86400),
      env.DB.prepare("DELETE FROM refresh_tokens WHERE expires_at < ?").bind(rfc3339(now)),
    ]);
    return issueTokens(env, base, ac.email, refresh);
  }
  if (grant === "refresh_token") {
    const old = form.get("refresh_token") ?? "";
    if (!old) return oauthError(400, "invalid_request", "missing refresh_token");
    // Deleting the row claims the one-time token atomically.
    const rt = await env.DB.prepare("DELETE FROM refresh_tokens WHERE token = ? RETURNING *").bind(await refreshKey(old)).first<any>();
    if (!rt) return oauthError(400, "invalid_grant", "unknown refresh token");
    if (rfc3339(now) > rt.expires_at) return oauthError(400, "invalid_grant", "refresh token expired");
    const refresh = randomToken(32);
    await env.DB.batch([
      env.DB.prepare("INSERT OR REPLACE INTO credentials (email, password) VALUES (?, ?)").bind(rt.email, rt.password),
      env.DB.prepare("INSERT INTO refresh_tokens (token, email, password, client_id, expires_at) VALUES (?, ?, ?, ?, ?)").bind(await refreshKey(refresh), rt.email, rt.password, rt.client_id, rfc3339(now + REFRESH_TTL)),
    ]);
    return issueTokens(env, base, rt.email, refresh);
  }
  return oauthError(400, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token");
}

// ---------------------------------------------------------------------------
// MCP endpoint (stateless streamable HTTP, JSON responses)
// ---------------------------------------------------------------------------

function unauthorized(base: string, error?: string): Response {
  const challenge = `Bearer resource_metadata="${base}/.well-known/oauth-protected-resource"` + (error ? `, error="invalid_token", error_description="${error}"` : "");
  return new Response(null, { status: 401, headers: { "WWW-Authenticate": challenge } });
}

async function credentialsFor(env: Env, header: string): Promise<{ email: string; password: string } | string> {
  if (header.startsWith("Basic ")) {
    try {
      const decoded = new TextDecoder().decode(Uint8Array.from(atob(header.slice(6).trim()), (c) => c.charCodeAt(0)));
      const i = decoded.indexOf(":");
      if (i >= 0) return { email: decoded.slice(0, i), password: decoded.slice(i + 1) };
    } catch {}
    return "malformed basic credentials";
  }
  if (header.startsWith("Bearer ")) {
    const claims = await parseJWT(env, header.slice(7).trim());
    if (!claims || typeof claims.sub !== "string" || !claims.sub) return "invalid or expired token";
    const row = await env.DB.prepare("SELECT password FROM credentials WHERE email = ?").bind(claims.sub).first<{ password: string }>();
    if (!row) return "no credentials found for user";
    try {
      return { email: claims.sub, password: await decryptPassword(env, row.password) };
    } catch {
      return "stored credential is unreadable";
    }
  }
  return "unsupported authorization scheme";
}

const rpcResult = (id: any, result: any) => ({ jsonrpc: "2.0", id, result });
const rpcError = (id: any, code: number, message: string) => ({ jsonrpc: "2.0", id, error: { code, message } });

function toolResult(r: { data?: any; error?: string }) {
  if (r.error !== undefined) return { content: [{ type: "text", text: r.error }], isError: true };
  return { content: [{ type: "text", text: JSON.stringify(r.data, null, 2) }], structuredContent: { data: r.data } };
}

async function callTool(env: Env, base: string, cred: { email: string; password: string }, name: string, args: Record<string, any>) {
  const stub = account(env, cred.email);
  try {
    if (name !== "things_diagnose") return toolResult((await stub.callTool(cred.email, cred.password, name, args)) as any);
    const r: { data?: any; error?: string } = (await stub.diagnose(cred.email, cred.password)) as any;
    if (r.error !== undefined) return toolResult(r);
    const id = crypto.randomUUID();
    const now = nowSec();
    try {
      await env.DB.batch([
        env.DB.prepare("INSERT INTO diagnoses (token, report_json, email, created_at) VALUES (?, ?, ?, ?)").bind(id, JSON.stringify(r.data), maskEmail(cred.email), rfc3339(now)),
        env.DB.prepare("DELETE FROM diagnoses WHERE created_at < ?").bind(rfc3339(now - DIAGNOSIS_TTL)),
      ]);
      r.data.shareUrl = base + "/d/" + id;
    } catch {}
    return toolResult(r);
  } catch (e: any) {
    console.error(`tool ${name} failed: ${e?.message ?? e}`);
    return toolResult({ error: `internal error: ${e?.message ?? e}` });
  }
}

function maskEmail(email: string): string {
  const i = email.indexOf("@");
  return i <= 0 ? "***" : email[0] + "***@" + email.slice(i + 1);
}

async function mcp(req: Request, env: Env, base: string): Promise<Response> {
  if (req.method !== "POST") return methodNotAllowed();
  const header = req.headers.get("Authorization") ?? "";
  if (!header) return unauthorized(base);
  const cred = await credentialsFor(env, header);
  if (typeof cred === "string") return unauthorized(base, cred);

  let msg: any;
  try {
    msg = await req.json();
  } catch {
    return json(400, rpcError(null, -32700, "Parse error"));
  }
  if (!msg || typeof msg !== "object" || Array.isArray(msg) || msg.jsonrpc !== "2.0" || typeof msg.method !== "string") {
    return json(400, rpcError(msg?.id ?? null, -32600, "Invalid Request"));
  }
  if (msg.id === undefined || msg.id === null) return new Response(null, { status: 202 }); // notification
  const id = msg.id;
  const params = msg.params ?? {};
  switch (msg.method) {
    case "initialize": {
      const requested = params.protocolVersion;
      const protocolVersion = PROTOCOL_VERSIONS.includes(requested) ? requested : "2025-06-18";
      return json(200, rpcResult(id, { protocolVersion, capabilities: { tools: {} }, serverInfo: SERVER_INFO, instructions: INSTRUCTIONS }));
    }
    case "ping":
      return json(200, rpcResult(id, {}));
    case "tools/list":
      return json(200, rpcResult(id, { tools: TOOLS }));
    case "tools/call": {
      const name = params.name;
      if (typeof name !== "string" || !TOOL_NAMES.has(name)) return json(200, rpcError(id, -32602, `tool '${name}' not found: tool not found`));
      const args = params.arguments && typeof params.arguments === "object" && !Array.isArray(params.arguments) ? params.arguments : {};
      return json(200, rpcResult(id, await callTool(env, base, cred, name, args)));
    }
    default:
      return json(200, rpcError(id, -32601, "Method not found"));
  }
}

// ---------------------------------------------------------------------------
// Router (static pages are served from public/ before the Worker runs)
// ---------------------------------------------------------------------------

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const url = new URL(req.url);
    const base = url.origin;
    const path = url.pathname;
    const get = (fn: () => Response | Promise<Response>) => (req.method === "GET" ? fn() : methodNotAllowed());
    const post = (fn: () => Response | Promise<Response>) => (req.method === "POST" ? fn() : methodNotAllowed());

    switch (path) {
      case "/mcp":
        return mcp(req, env, base);
      case "/.well-known/oauth-protected-resource":
      case "/.well-known/oauth-protected-resource/mcp":
        return get(() => protectedResourceMetadata(base));
      case "/.well-known/oauth-authorization-server":
      case "/.well-known/oauth-authorization-server/mcp":
        return get(() => authServerMetadata(base));
      case "/register":
        return post(() => register(req, env));
      case "/authorize":
        return req.method === "GET" || req.method === "POST" ? authorize(req, env, url) : methodNotAllowed();
      case "/token":
        return post(() => token(req, env, base));
    }
    const m = path.match(/^\/d\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i);
    if (m) {
      if (req.method !== "GET") return methodNotAllowed();
      const row = await env.DB.prepare("SELECT report_json FROM diagnoses WHERE token = ? AND created_at >= ?").bind(m[1].toLowerCase(), rfc3339(nowSec() - DIAGNOSIS_TTL)).first<{ report_json: string }>();
      if (row) return new Response(row.report_json, { headers: { "Content-Type": "application/json; charset=utf-8" } });
    }
    return new Response("404 page not found\n", { status: 404, headers: { "Content-Type": "text/plain; charset=utf-8" } });
  },
} satisfies ExportedHandler<Env>;
