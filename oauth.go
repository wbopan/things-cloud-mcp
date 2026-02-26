package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

// ---------------------------------------------------------------------------
// OAuth 2.1 types
// ---------------------------------------------------------------------------

type OAuthServer struct {
	um            *UserManager
	jwtSecret     []byte
	clients       map[string]*OAuthClient    // client_id -> client
	authCodes     map[string]*AuthCode        // code -> auth code data
	refreshTokens map[string]*RefreshToken    // token -> refresh data
	credentials   map[string]string           // email -> password (from successful authorizations)
	mu            sync.RWMutex
}

type OAuthClient struct {
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret,omitempty"`
	ClientName    string   `json:"client_name"`
	RedirectURIs  []string `json:"redirect_uris"`
	GrantTypes    []string `json:"grant_types"`
	ResponseTypes []string `json:"response_types"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

type AuthCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	Email         string
	Password      string
	CodeChallenge string
	ExpiresAt     time.Time
	Used          bool
}

type RefreshToken struct {
	Token     string
	Email     string
	Password  string
	ClientID  string
	ExpiresAt time.Time
}

// NewOAuthServer creates a new OAuth server with the given UserManager and JWT secret.
func NewOAuthServer(um *UserManager, jwtSecret []byte) *OAuthServer {
	return &OAuthServer{
		um:            um,
		jwtSecret:     jwtSecret,
		clients:       make(map[string]*OAuthClient),
		authCodes:     make(map[string]*AuthCode),
		refreshTokens: make(map[string]*RefreshToken),
		credentials:   make(map[string]string),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func getBaseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		host := r.Host
		if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "[::1]") {
			scheme = "http"
		} else {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host
}

func verifyPKCE(codeVerifier, codeChallenge string) bool {
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == codeChallenge
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, errCode, desc string) {
	writeJSON(w, status, map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

// ---------------------------------------------------------------------------
// Minimal JWT HS256 implementation
// ---------------------------------------------------------------------------

func (o *OAuthServer) createJWT(claims map[string]any) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, o.jwtSecret)
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

func (o *OAuthServer) parseJWT(tokenStr string) (map[string]any, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, o.jwtSecret)
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid JWT signature")
	}

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT payload encoding: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid JWT payload: %w", err)
	}

	// Check expiration
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("JWT expired")
		}
	}

	return claims, nil
}

// ResolveBearer validates a Bearer token and returns the user's ThingsMCP instance.
func (o *OAuthServer) ResolveBearer(token string) (string, string, error) {
	claims, err := o.parseJWT(token)
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}

	email, ok := claims["sub"].(string)
	if !ok || email == "" {
		return "", "", fmt.Errorf("invalid token: missing subject")
	}

	o.mu.RLock()
	password, ok := o.credentials[email]
	o.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("no credentials found for user")
	}

	return email, password, nil
}

// ---------------------------------------------------------------------------
// Discovery endpoints
// ---------------------------------------------------------------------------

func (o *OAuthServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := getBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                base,
		"authorization_servers":   []string{base},
		"scopes_supported":       []string{"things:manage"},
		"bearer_methods_supported": []string{"header"},
	})
}

func (o *OAuthServer) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := getBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                base,
		"authorization_endpoint":                base + "/authorize",
		"token_endpoint":                        base + "/token",
		"registration_endpoint":                 base + "/register",
		"scopes_supported":                      []string{"things:manage"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"token_endpoint_auth_methods_supported": []string{"none", "client_secret_basic"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

// ---------------------------------------------------------------------------
// Dynamic client registration
// ---------------------------------------------------------------------------

func (o *OAuthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientName    string   `json:"client_name"`
		RedirectURIs  []string `json:"redirect_uris"`
		GrantTypes    []string `json:"grant_types"`
		ResponseTypes []string `json:"response_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if len(req.RedirectURIs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "redirect_uris is required")
		return
	}

	if req.ClientName == "" {
		req.ClientName = "Unknown Client"
	}
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code"}
	}
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{"code"}
	}

	clientID := randomString(24)

	client := &OAuthClient{
		ClientID:      clientID,
		ClientName:    req.ClientName,
		RedirectURIs:  req.RedirectURIs,
		GrantTypes:    req.GrantTypes,
		ResponseTypes: req.ResponseTypes,
		CreatedAt:     time.Now(),
	}

	o.mu.Lock()
	o.clients[clientID] = client
	o.mu.Unlock()

	log.Printf("OAuth: registered client %q (id=%s)", req.ClientName, clientID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":      clientID,
		"client_name":    req.ClientName,
		"redirect_uris":  req.RedirectURIs,
		"grant_types":    req.GrantTypes,
		"response_types": req.ResponseTypes,
	})
}

// ---------------------------------------------------------------------------
// Authorization endpoint
// ---------------------------------------------------------------------------

func (o *OAuthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		o.handleAuthorizeGet(w, r)
	case http.MethodPost:
		o.handleAuthorizePost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (o *OAuthServer) handleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	responseType := q.Get("response_type")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	// Validate required params
	if responseType != "code" {
		o.renderLoginPage(w, "", "Unsupported response_type. Must be 'code'.", q.Encode())
		return
	}
	if clientID == "" || redirectURI == "" || state == "" {
		o.renderLoginPage(w, "", "Missing required parameters: client_id, redirect_uri, state.", q.Encode())
		return
	}
	if codeChallenge == "" || codeChallengeMethod != "S256" {
		o.renderLoginPage(w, "", "PKCE required: code_challenge and code_challenge_method=S256.", q.Encode())
		return
	}

	o.mu.RLock()
	client, ok := o.clients[clientID]
	o.mu.RUnlock()
	if !ok {
		o.renderLoginPage(w, "", "Unknown client_id.", q.Encode())
		return
	}

	// Validate redirect URI
	validURI := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			validURI = true
			break
		}
	}
	if !validURI {
		o.renderLoginPage(w, "", "Invalid redirect_uri.", q.Encode())
		return
	}

	o.renderLoginPage(w, client.ClientName, "", q.Encode())
}

func (o *OAuthServer) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Query params come from the URL; form values from POST body
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")

	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	if email == "" || password == "" {
		o.mu.RLock()
		client := o.clients[clientID]
		o.mu.RUnlock()
		clientName := ""
		if client != nil {
			clientName = client.ClientName
		}
		o.renderLoginPage(w, clientName, "Email and password are required.", q.Encode())
		return
	}

	// Verify Things Cloud credentials
	c := thingscloud.New(thingscloud.APIEndpoint, email, password)
	if _, err := c.Verify(); err != nil {
		o.mu.RLock()
		client := o.clients[clientID]
		o.mu.RUnlock()
		clientName := ""
		if client != nil {
			clientName = client.ClientName
		}
		o.renderLoginPage(w, clientName, "Invalid Things Cloud credentials.", q.Encode())
		return
	}

	// Generate auth code
	code := randomString(32)
	authCode := &AuthCode{
		Code:          code,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		Email:         email,
		Password:      password,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(10 * time.Minute),
		Used:          false,
	}

	o.mu.Lock()
	o.authCodes[code] = authCode
	o.credentials[email] = password
	o.mu.Unlock()

	log.Printf("OAuth: auth code issued for %s (client=%s)", email, clientID)

	// Redirect to client
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	location := redirectURI + sep + "code=" + code + "&state=" + state
	o.renderSuccessPage(w, location)
}

func (o *OAuthServer) renderSuccessPage(w http.ResponseWriter, redirectURL string) {
	html := `<!DOCTYPE html><html><head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Authorized - Things Cloud MCP</title>
<style>
` + sharedCSS + `
.auth-container{
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  padding:24px;
}
.auth-card{
  width:100%;
  max-width:380px;
  background:var(--surface);
  border:1px solid var(--divider);
  border-radius:var(--radius);
  padding:40px 32px;
  box-shadow:0 2px 12px rgba(0,0,0,0.06);
  text-align:center;
}
.auth-icon{
  display:flex;
  justify-content:center;
  margin-bottom:24px;
}
.auth-title{
  font-size:20px;
  font-weight:700;
  letter-spacing:-0.3px;
  margin-bottom:8px;
}
.auth-subtitle{
  font-size:14px;
  color:var(--text-secondary);
  line-height:1.5;
}
@keyframes spin{to{transform:rotate(360deg)}}
.spinner{
  width:20px;height:20px;
  border:2px solid var(--divider);
  border-top-color:var(--blue);
  border-radius:50%;
  animation:spin 0.8s linear infinite;
  display:inline-block;
  vertical-align:middle;
  margin-right:8px;
}
</style>
</head>
<body>
<div class="auth-container">
  <div class="auth-card">
    <div class="auth-icon">` + `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="48" height="48" fill="none">
  <circle cx="32" cy="32" r="28" fill="#34C759"/>
  <polyline points="20,33 28,41 44,25" stroke="#fff" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>` + `</div>
    <div class="auth-title">Authorization Successful</div>
    <div class="auth-subtitle">You can close this window.</div>
  </div>
</div>
<script>setTimeout(function(){window.location.href="` + "{{redirect_url}}" + `"},1500);</script>
</body></html>`
	html = strings.Replace(html, "{{redirect_url}}", redirectURL, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ---------------------------------------------------------------------------
// Token endpoint
// ---------------------------------------------------------------------------

func (o *OAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "invalid form body")
		return
	}

	grantType := r.PostFormValue("grant_type")
	switch grantType {
	case "authorization_code":
		o.handleAuthCodeGrant(w, r)
	case "refresh_token":
		o.handleRefreshTokenGrant(w, r)
	default:
		writeJSONError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (o *OAuthServer) handleAuthCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.PostFormValue("code")
	clientID := r.PostFormValue("client_id")
	redirectURI := r.PostFormValue("redirect_uri")
	codeVerifier := r.PostFormValue("code_verifier")

	if code == "" || clientID == "" || redirectURI == "" || codeVerifier == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "missing required parameters: code, client_id, redirect_uri, code_verifier")
		return
	}

	o.mu.Lock()
	ac, ok := o.authCodes[code]
	if !ok {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "unknown authorization code")
		return
	}
	if ac.Used {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "authorization code already used")
		return
	}
	if time.Now().After(ac.ExpiresAt) {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "authorization code expired")
		return
	}
	if ac.ClientID != clientID {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	if ac.RedirectURI != redirectURI {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	// Verify PKCE
	if !verifyPKCE(codeVerifier, ac.CodeChallenge) {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	ac.Used = true
	email := ac.Email
	password := ac.Password

	// Store credentials for Bearer token resolution
	o.credentials[email] = password
	o.mu.Unlock()

	// Generate tokens
	base := getBaseURL(r)
	accessToken, err := o.createJWT(map[string]any{
		"sub":   email,
		"iss":   base,
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"scope": "things:manage",
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "server_error", "failed to create access token")
		return
	}

	refreshTok := randomString(32)
	o.mu.Lock()
	o.refreshTokens[refreshTok] = &RefreshToken{
		Token:     refreshTok,
		Email:     email,
		Password:  password,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // 30 days
	}
	o.mu.Unlock()

	log.Printf("OAuth: tokens issued for %s", email)

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": refreshTok,
		"scope":         "things:manage",
	})
}

func (o *OAuthServer) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshTok := r.PostFormValue("refresh_token")
	if refreshTok == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "missing refresh_token")
		return
	}

	o.mu.Lock()
	rt, ok := o.refreshTokens[refreshTok]
	if !ok {
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "unknown refresh token")
		return
	}
	if time.Now().After(rt.ExpiresAt) {
		delete(o.refreshTokens, refreshTok)
		o.mu.Unlock()
		writeJSONError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
		return
	}

	email := rt.Email
	password := rt.Password
	clientID := rt.ClientID

	// Delete old refresh token
	delete(o.refreshTokens, refreshTok)

	// Store credentials
	o.credentials[email] = password
	o.mu.Unlock()

	// Generate new tokens
	base := getBaseURL(r)
	accessToken, err := o.createJWT(map[string]any{
		"sub":   email,
		"iss":   base,
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"scope": "things:manage",
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "server_error", "failed to create access token")
		return
	}

	newRefreshTok := randomString(32)
	o.mu.Lock()
	o.refreshTokens[newRefreshTok] = &RefreshToken{
		Token:     newRefreshTok,
		Email:     email,
		Password:  password,
		ClientID:  clientID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	o.mu.Unlock()

	log.Printf("OAuth: tokens refreshed for %s", email)

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": newRefreshTok,
		"scope":         "things:manage",
	})
}

// ---------------------------------------------------------------------------
// Login page HTML
// ---------------------------------------------------------------------------

var authorizePageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Sign In - Things Cloud MCP</title>
<style>
` + sharedCSS + `
.auth-container{
  min-height:100vh;
  display:flex;
  align-items:center;
  justify-content:center;
  padding:24px;
}
.auth-card{
  width:100%;
  max-width:380px;
  background:var(--surface);
  border:1px solid var(--divider);
  border-radius:var(--radius);
  padding:40px 32px;
  box-shadow:0 2px 12px rgba(0,0,0,0.06);
}
@media(prefers-color-scheme:dark){
  .auth-card{
    box-shadow:0 2px 12px rgba(0,0,0,0.2);
  }
}
.auth-icon{
  display:flex;
  justify-content:center;
  margin-bottom:24px;
}
.auth-icon svg{width:48px;height:48px}
.auth-title{
  font-size:20px;
  font-weight:700;
  text-align:center;
  letter-spacing:-0.3px;
  margin-bottom:6px;
}
.auth-subtitle{
  font-size:14px;
  color:var(--text-secondary);
  text-align:center;
  margin-bottom:28px;
}
.auth-error{
  background:#FFF2F2;
  color:#D70015;
  border:1px solid #FFD6D6;
  border-radius:var(--radius-sm);
  padding:10px 14px;
  font-size:13px;
  margin-bottom:20px;
  line-height:1.45;
}
@media(prefers-color-scheme:dark){
  .auth-error{
    background:#3A1C1C;
    color:#FF6B6B;
    border-color:#5A2C2C;
  }
}
.auth-field{
  margin-bottom:16px;
}
.auth-field label{
  display:block;
  font-size:13px;
  font-weight:600;
  margin-bottom:6px;
  color:var(--text);
}
.auth-field input{
  width:100%;
  padding:10px 14px;
  font-size:15px;
  border:1px solid var(--divider);
  border-radius:var(--radius-sm);
  background:var(--bg);
  color:var(--text);
  font-family:inherit;
  outline:none;
  transition:border-color 0.15s;
  box-sizing:border-box;
}
.auth-field input:focus{
  border-color:var(--blue);
}
.auth-btn{
  width:100%;
  padding:12px;
  font-size:15px;
  font-weight:600;
  color:#fff;
  background:var(--blue);
  border:none;
  border-radius:var(--radius-sm);
  cursor:pointer;
  font-family:inherit;
  transition:background 0.15s;
  margin-top:8px;
}
.auth-btn:hover{
  background:var(--blue-hover);
}
</style>
</head>
<body>
<div class="auth-container">
  <div class="auth-card">
    <div class="auth-icon">` + `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="48" height="48" fill="none">
  <path d="M50 46a11 11 0 0 0 0-22 11 11 0 0 0-1-.04 15 15 0 0 0-29-2A13 13 0 0 0 14 46h36z" fill="#1A7CF9"/>
  <polyline points="24,34 30,40 42,28" stroke="#fff" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
</svg>` + `</div>
    <div class="auth-title">Sign in with Things Cloud</div>
    <div class="auth-subtitle">{{subtitle}}</div>
    {{error}}
    <form method="POST" action="/authorize?{{query}}">
      <div class="auth-field">
        <label for="email">Email</label>
        <input type="email" id="email" name="email" required autocomplete="email" autofocus>
      </div>
      <div class="auth-field">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required autocomplete="current-password">
      </div>
      <button type="submit" class="auth-btn">Authorize</button>
    </form>
  </div>
</div>
</body>
</html>`

func (o *OAuthServer) renderLoginPage(w http.ResponseWriter, clientName, errMsg, queryString string) {
	subtitle := "Authorize access to your tasks"
	if clientName != "" {
		subtitle = "<strong>" + htmlEscape(clientName) + "</strong> wants to access your tasks"
	}

	errorHTML := ""
	if errMsg != "" {
		errorHTML = `<div class="auth-error">` + htmlEscape(errMsg) + `</div>`
	}

	html := strings.Replace(
		strings.Replace(
			strings.Replace(authorizePageHTML, "{{subtitle}}", subtitle, 1),
			"{{error}}", errorHTML, 1),
		"{{query}}", queryString, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
