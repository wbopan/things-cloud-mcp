-- OAuth state (same tables as the Go server's oauth.db, plus auth_codes,
-- which the Go server kept in memory).
CREATE TABLE IF NOT EXISTS clients (
  client_id TEXT PRIMARY KEY, client_secret TEXT DEFAULT '',
  client_name TEXT NOT NULL, redirect_uris TEXT NOT NULL,
  grant_types TEXT NOT NULL, response_types TEXT NOT NULL,
  created_at TEXT NOT NULL
);
-- token is "sha256:<hex>"; password is AES-GCM "enc:v1:..."; expires_at is RFC 3339 UTC.
CREATE TABLE IF NOT EXISTS refresh_tokens (
  token TEXT PRIMARY KEY, email TEXT NOT NULL, password TEXT NOT NULL,
  client_id TEXT NOT NULL, expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS credentials (email TEXT PRIMARY KEY, password TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS diagnoses (
  token TEXT PRIMARY KEY, report_json TEXT NOT NULL,
  email TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS auth_codes (
  code TEXT PRIMARY KEY, client_id TEXT NOT NULL, redirect_uri TEXT NOT NULL,
  email TEXT NOT NULL, password TEXT NOT NULL, code_challenge TEXT NOT NULL,
  expires_at INTEGER NOT NULL, used INTEGER NOT NULL DEFAULT 0
);
