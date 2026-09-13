package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOAuthCredentialsAreEncryptedAtRestAndReloaded(t *testing.T) {
	t.Setenv("CREDENTIALS_SECRET", "")
	dir := t.TempDir()
	o := NewOAuthServer(NewUserManager(), dir)
	const email = "person@example.com"
	const password = "dummy-test-password"
	if err := o.persistCredential(email, password); err != nil {
		t.Fatalf("persist credential: %v", err)
	}
	refresh := &RefreshToken{
		Token: "refresh-test", Email: email, Password: password,
		ClientID: "client-test", ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := o.persistRefreshToken(refresh); err != nil {
		t.Fatalf("persist refresh token: %v", err)
	}

	var storedCredential, storedRefresh string
	if err := o.db.QueryRow(`SELECT password FROM credentials WHERE email=?`, email).Scan(&storedCredential); err != nil {
		t.Fatalf("read stored credential: %v", err)
	}
	if err := o.db.QueryRow(`SELECT password FROM refresh_tokens WHERE token=?`, refreshTokenKey(refresh.Token)).Scan(&storedRefresh); err != nil {
		t.Fatalf("read stored refresh credential: %v", err)
	}
	for name, stored := range map[string]string{"credential": storedCredential, "refresh": storedRefresh} {
		if stored == password || !strings.HasPrefix(stored, encryptedPasswordPrefix) {
			t.Fatalf("%s was not encrypted at rest", name)
		}
	}
	if err := o.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reloaded := NewOAuthServer(NewUserManager(), dir)
	defer reloaded.db.Close()
	if got := reloaded.credentials[email]; got != password {
		t.Fatalf("reloaded credential mismatch")
	}
	if got := reloaded.refreshTokens[refreshTokenKey(refresh.Token)]; got == nil || got.Password != password {
		t.Fatalf("reloaded refresh credential mismatch")
	}
	var storedToken string
	if err := reloaded.db.QueryRow(`SELECT token FROM refresh_tokens LIMIT 1`).Scan(&storedToken); err != nil {
		t.Fatalf("read stored refresh token: %v", err)
	}
	if storedToken == refresh.Token || !strings.HasPrefix(storedToken, refreshTokenHashPrefix) {
		t.Fatal("refresh token was not hashed at rest")
	}

	for _, path := range []string{dir, dir + "/oauth.db", dir + "/credentials.key"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("permissions for %s = %o, want %o", path, got, want)
		}
	}
}

func TestOAuthMigratesLegacyPlaintextCredentials(t *testing.T) {
	t.Setenv("CREDENTIALS_SECRET", "")
	dir := t.TempDir()
	o := NewOAuthServer(NewUserManager(), dir)
	const email = "legacy@example.com"
	const password = "legacy-dummy-password"
	if _, err := o.db.Exec(`INSERT INTO credentials(email, password) VALUES(?, ?)`, email, password); err != nil {
		t.Fatalf("insert legacy credential: %v", err)
	}
	const rawRefresh = "legacy-raw-refresh-token"
	if _, err := o.db.Exec(`INSERT INTO refresh_tokens VALUES(?,?,?,?,?)`, rawRefresh, email, password, "legacy-client", time.Now().Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("insert legacy refresh token: %v", err)
	}
	if err := o.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	migrated := NewOAuthServer(NewUserManager(), dir)
	defer migrated.db.Close()
	if got := migrated.credentials[email]; got != password {
		t.Fatalf("legacy credential was not loaded")
	}
	var stored string
	if err := migrated.db.QueryRow(`SELECT password FROM credentials WHERE email=?`, email).Scan(&stored); err != nil {
		t.Fatalf("read migrated credential: %v", err)
	}
	if stored == password || !strings.HasPrefix(stored, encryptedPasswordPrefix) {
		t.Fatal("legacy credential remained plaintext")
	}
	var storedToken, storedRefreshPassword string
	if err := migrated.db.QueryRow(`SELECT token, password FROM refresh_tokens LIMIT 1`).Scan(&storedToken, &storedRefreshPassword); err != nil {
		t.Fatalf("read migrated refresh token: %v", err)
	}
	if storedToken != refreshTokenKey(rawRefresh) || storedRefreshPassword == password || !strings.HasPrefix(storedRefreshPassword, encryptedPasswordPrefix) {
		t.Fatal("legacy refresh token or credential was not migrated")
	}
	if got := migrated.refreshTokens[refreshTokenKey(rawRefresh)]; got == nil || got.Password != password {
		t.Fatal("migrated refresh token was not loaded")
	}
}

// newBearerTestServer builds an OAuth server holding one authorized user, so a
// token can be minted for it without going through the whole authorize flow.
func newBearerTestServer(t *testing.T, email, password string) *OAuthServer {
	t.Helper()
	t.Setenv("CREDENTIALS_SECRET", "")
	o := NewOAuthServer(NewUserManager(), t.TempDir())
	t.Cleanup(func() { o.db.Close() })
	if err := o.persistCredential(email, password); err != nil {
		t.Fatalf("persist credential: %v", err)
	}
	o.credentials[email] = password
	return o
}

func (o *OAuthServer) testToken(t *testing.T, email string, exp time.Time) string {
	t.Helper()
	token, err := o.createJWT(map[string]any{
		"sub": email, "iss": "https://example.test",
		"exp": exp.Unix(), "iat": time.Now().Unix(), "scope": "things:manage",
	})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return token
}

// A bad access token must fail at the transport with 401 and a challenge, never
// inside a tool handler. A handler's error leaves as HTTP 200 with isError, and
// a client cannot read that as "re-authenticate": it keeps replaying the dead
// token, never runs the refresh grant, and the untouched refresh token ages out.
// That is how a single expired hour turns into a manual re-authorization.
func TestInvalidBearerIsRejectedWithChallengeBeforeReachingHandlers(t *testing.T) {
	const email, password = "person@example.com", "dummy-test-password"
	o := newBearerTestServer(t, email, password)

	for _, tc := range []struct{ name, header string }{
		{"expired token", "Bearer " + o.testToken(t, email, time.Now().Add(-time.Minute))},
		{"forged signature", "Bearer not.a.jwt"},
		{"unknown subject", "Bearer " + o.testToken(t, "stranger@example.com", time.Now().Add(time.Hour))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			handler := requireBearer(o, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached = true
			}))

			req := httptest.NewRequest(http.MethodPost, "https://example.test/mcp", nil)
			req.Header.Set("Authorization", tc.header)
			rec := httptest.NewRecorder()
			handler(rec, req)

			if reached {
				t.Fatal("request reached the MCP handler; the error would leave as HTTP 200 and no refresh would run")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			challenge := rec.Header().Get("WWW-Authenticate")
			if !strings.Contains(challenge, `error="invalid_token"`) {
				t.Fatalf("challenge %q carries no invalid_token error; clients key their refresh on it", challenge)
			}
			if !strings.Contains(challenge, `resource_metadata="https://example.test/.well-known/oauth-protected-resource"`) {
				t.Fatalf("challenge %q does not point at the resource metadata", challenge)
			}
		})
	}
}

// The guard must not become a second gate that locks out what used to work: a
// valid token and CLI-style Basic auth both have to pass through.
func TestRequireBearerPassesValidTokenAndBasicAuth(t *testing.T) {
	const email, password = "person@example.com", "dummy-test-password"
	o := newBearerTestServer(t, email, password)

	basic := base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
	for _, tc := range []struct{ name, header string }{
		{"valid token", "Bearer " + o.testToken(t, email, time.Now().Add(time.Hour))},
		{"basic auth", "Basic " + basic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			handler := requireBearer(o, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
			}))
			req := httptest.NewRequest(http.MethodPost, "https://example.test/mcp", nil)
			req.Header.Set("Authorization", tc.header)
			handler(httptest.NewRecorder(), req)
			if !reached {
				t.Fatal("request was rejected but should have been served")
			}
		})
	}
}

// Without an Authorization header the challenge stays a bare pointer to the
// metadata: nothing was presented, so nothing can be invalid_token.
func TestMissingAuthorizationGetsBareChallenge(t *testing.T) {
	o := newBearerTestServer(t, "person@example.com", "dummy-test-password")
	rec := httptest.NewRecorder()
	requireBearer(o, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached the MCP handler")
	}))(rec, httptest.NewRequest(http.MethodPost, "https://example.test/mcp", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if challenge := rec.Header().Get("WWW-Authenticate"); strings.Contains(challenge, "error=") {
		t.Fatalf("challenge %q reports an error although no token was presented", challenge)
	}
}

// The error text lands inside a quoted-string, so it must not be able to carry
// a quote or a newline out of the header.
func TestChallengeReasonCannotBreakOutOfTheHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	writeBearerChallenge(rec, "https://example.test", "bad\r\nX-Injected: 1 \"quote\"")
	challenge := rec.Header().Get("WWW-Authenticate")
	for _, forbidden := range []string{"\r", "\n", `"quote"`} {
		if strings.Contains(challenge, forbidden) {
			t.Fatalf("challenge %q still carries %q", challenge, forbidden)
		}
	}
	if !strings.Contains(challenge, `resource_metadata=`) {
		t.Fatalf("challenge %q lost its metadata pointer", challenge)
	}
}
