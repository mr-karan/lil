package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mr-karan/lil/internal/auth"
	"github.com/mr-karan/lil/internal/redirect"
	"github.com/mr-karan/lil/internal/store"
)

type testUser struct {
	user  store.User
	actor store.Actor
	token string
}

type testEnv struct {
	app    *App
	router http.Handler
	store  *store.Store
	idpURL string
}

// newTestEnv builds the real router in oidc mode. The IdP only needs to serve
// discovery: users and tokens are created through the store.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 "http://" + r.Host,
			"authorization_endpoint": "http://" + r.Host + "/authorize",
			"token_endpoint":         "http://" + r.Host + "/token",
			"jwks_uri":               "http://" + r.Host + "/jwks",
		})
	}))
	t.Cleanup(idp.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := store.New(store.Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	authn, err := auth.New(t.Context(), auth.Config{
		Mode: auth.ModeOIDC, IssuerURL: idp.URL, ClientID: "client-id", ClientSecret: "secret",
		RedirectURL: "http://127.0.0.1:7000/auth/oidc", AllowedEmails: []string{"alice@example.com", "bob@example.com"}, AllowedDomains: []string{"example.com"},
	}, s, logger)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: s, logger: logger}
	return &testEnv{app: app, router: app.routes(authn), store: s, idpURL: idp.URL}
}

func (e *testEnv) newUser(t *testing.T, email string) testUser {
	t.Helper()
	user, err := e.store.UpsertOIDCUser(t.Context(), e.idpURL, "sub-"+email, email, "")
	if err != nil {
		t.Fatal(err)
	}
	actor := store.Actor{UserID: user.ID}
	token, _, err := e.store.CreateAPIToken(t.Context(), actor, "test")
	if err != nil {
		t.Fatal(err)
	}
	return testUser{user: user, actor: actor, token: token}
}

func (e *testEnv) request(method, path, body, token string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
	return w
}

func dataOf[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Status string `json:"status"`
		Data   T      `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Status != "success" {
		t.Fatalf("bad envelope (%d): %s", w.Code, w.Body.String())
	}
	return envelope.Data
}

func TestHTTPRedirectsAndManagement(t *testing.T) {
	env := newTestEnv(t)
	router := env.router
	alice := env.newUser(t, "alice@example.com")
	ios := "https://apps.apple.com/app/id123456789?action=write-review"
	android := "https://play.google.com/store/apps/details?id=com.example.app"
	if _, err := env.store.CreateShortURL(context.Background(), alice.actor, "https://example.com/choose", "", "rate-us", 0, map[string]string{"ios": ios, "android": android}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, path, ua, want string
		status                 int
	}{
		{"GET", "/rate-us", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", ios, 302},
		{"GET", "/rate-us", "Mozilla/5.0 (Linux; Android 14)", android, 302},
		{"GET", "/rate-us?platform=ios", "", ios, 302},
		{"GET", "/rate-us?platform=web", "", "https://example.com/choose", 302},
		{"HEAD", "/rate-us?platform=android", "", android, 302},
		{"GET", "/rate-us?platform=bad", "", "", 400},
		{"GET", "/missing", "", "", 404},
		{"GET", "/admin", "", "/admin/", 307},
		{"GET", "/api/v1/urls", "", "", 401},
		{"GET", "/api/v1/health", "", "", 200},
	} {
		t.Run(tt.method+tt.path+tt.ua, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			r.Header.Set("User-Agent", tt.ua)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tt.status || w.Header().Get("Location") != tt.want {
				t.Fatalf("%d %s", w.Code, w.Header().Get("Location"))
			}
			if tt.status == 302 && (!strings.Contains(w.Header().Get("Cache-Control"), "no-store") || !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "User-Agent")) {
				t.Fatal(w.Header())
			}
			if tt.status == 302 && w.Header().Get("X-Lil-Platform") == "" {
				t.Fatal("missing detected platform header")
			}
		})
	}
	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"url":"javascript:alert(1)"}`, 400},
		{`{"url":"https://example.com","slug":"admin"}`, 400},
		{`{"url":"https://example.com","device_urls":{"windows":"https://example.com"}}`, 400},
		{`{"url":"https://example.com","slug":"rate-us"}`, 409},
		{`{"url":"https://example.com","slug":"new-link"}`, 200},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(tt.body))
		r.Header.Set("Authorization", "Bearer "+alice.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.body, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	r.Header.Set("Authorization", "Bearer "+alice.token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-JSON content type: %d", w.Code)
	}
}

func TestRedirectLogDoesNotExposeQueryValues(t *testing.T) {
	var output bytes.Buffer
	app := App{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	r := httptest.NewRequest(http.MethodGet, "/review", nil)
	r.Header.Set("User-Agent", "ExampleApp/1")
	app.logRedirectDecision(r, "review", "https://apps.example.com/app/id123?action=sensitive-review&token=secret", redirect.IOS, redirect.SourceUserAgent)

	logLine := output.String()
	for _, want := range []string{`"msg":"redirect selected"`, `"platform":"ios"`, `"detection_source":"user_agent"`, `"target_query_keys":["action","token"]`, `"target_url_sha256":`} {
		if !strings.Contains(logLine, want) {
			t.Errorf("log does not contain %s: %s", want, logLine)
		}
	}
	for _, secret := range []string{"sensitive-review", "secret"} {
		if strings.Contains(logLine, secret) {
			t.Errorf("log exposed query value %q: %s", secret, logLine)
		}
	}
}

func TestAuthenticationBoundary(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")

	w := env.request("GET", "/admin/", "", "")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/auth/login?next=%2Fadmin%2F" {
		t.Fatalf("admin without auth: %d %s", w.Code, w.Header().Get("Location"))
	}
	w = env.request("GET", "/api/v1/urls", "", "")
	if w.Code != http.StatusUnauthorized || w.Body.String() != `{"status":"error","message":"authentication required"}` {
		t.Fatalf("api without auth: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/v1/me", "/api/v1/users", "/api/v1/tokens", "/api/v1/audit", "/api/v1/metrics"} {
		if w := env.request("GET", path, "", ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without auth: %d", path, w.Code)
		}
		if w := env.request("GET", path, "", alice.token); w.Code != http.StatusOK {
			t.Fatalf("%s with token: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := env.request("GET", "/api/v1/urls", "", "lil_invalid"); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/api/v1/urls", nil)
	r.SetBasicAuth("admin", "password")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("basic auth must no longer work: %d", rec.Code)
	}
	if w := env.request("GET", "/api/v1/health", "", ""); w.Code != http.StatusOK {
		t.Fatalf("health: %d", w.Code)
	}
	if w := env.request("GET", "/auth/signed-out", "", ""); w.Code != http.StatusOK {
		t.Fatalf("signed-out page: %d", w.Code)
	}
	if w := env.request("GET", "/auth/login", "", ""); w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), env.idpURL+"/authorize?") {
		t.Fatalf("login: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestPublicRoutesDoNotSetSessionCookies(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")
	if _, err := env.store.CreateShortURL(t.Context(), alice.actor, "https://example.com", "", "public", 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/public", "/api/v1/health", "/missing"} {
		if w := env.request("GET", path, "", ""); len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s set cookies: %v", path, w.Header())
		}
	}
}

func TestCrossOriginRequestsAreRejected(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")
	const body = `{"url":"https://example.com"}`
	for _, tt := range []struct {
		name    string
		headers []string
		status  int
	}{
		{"sec-fetch-site cross-site", []string{"Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
		{"foreign origin", []string{"Origin", "https://evil.example"}, http.StatusForbidden},
		{"foreign origin and cross-site", []string{"Origin", "https://evil.example", "Sec-Fetch-Site", "cross-site"}, http.StatusForbidden},
		{"same-origin", []string{"Sec-Fetch-Site", "same-origin"}, http.StatusOK},
		{"no browser headers", nil, http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if w := env.request("POST", "/api/v1/shorten", body, alice.token, tt.headers...); w.Code != tt.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	if w := env.request("POST", "/auth/logout", "", "", "Sec-Fetch-Site", "cross-site"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-site logout: %d", w.Code)
	}
	if w := env.request("GET", "/api/v1/me", "", alice.token, "Sec-Fetch-Site", "cross-site"); w.Code != http.StatusOK {
		t.Fatalf("safe methods stay allowed: %d", w.Code)
	}
}

func TestUserManagementAPI(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")
	bob := env.newUser(t, "bob@example.com")
	path := func(u testUser, action string) string {
		return "/api/v1/users/" + strconv.FormatInt(int64(u.user.ID), 10) + "/" + action
	}

	me := dataOf[store.User](t, env.request("GET", "/api/v1/me", "", alice.token))
	if me.Email != "alice@example.com" || me.ID != alice.user.ID {
		t.Fatalf("me: %+v", me)
	}
	if users := dataOf[[]store.User](t, env.request("GET", "/api/v1/users", "", alice.token)); len(users) != 2 || users[0].Email != "alice@example.com" {
		t.Fatalf("users: %+v", users)
	}

	if w := env.request("POST", path(alice, "disable"), "", alice.token); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "you cannot disable yourself") {
		t.Fatalf("self-disable: %d %s", w.Code, w.Body.String())
	}
	if w := env.request("POST", "/api/v1/users/9999/disable", "", alice.token); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if w := env.request("POST", "/api/v1/users/abc/disable", "", alice.token); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}

	if w := env.request("GET", "/api/v1/me", "", bob.token); w.Code != http.StatusOK {
		t.Fatalf("bob before disable: %d", w.Code)
	}
	if w := env.request("POST", path(bob, "disable"), "", alice.token); w.Code != http.StatusNoContent {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}
	if w := env.request("GET", "/api/v1/me", "", bob.token); w.Code != http.StatusUnauthorized {
		t.Fatalf("bob token after disable must fail immediately: %d", w.Code)
	}
	users := dataOf[[]store.User](t, env.request("GET", "/api/v1/users", "", alice.token))
	if users[1].DisabledAt == nil {
		t.Fatalf("bob not shown as disabled: %+v", users[1])
	}
	if w := env.request("POST", path(bob, "enable"), "", alice.token); w.Code != http.StatusNoContent {
		t.Fatalf("enable: %d", w.Code)
	}
	if w := env.request("GET", "/api/v1/me", "", bob.token); w.Code != http.StatusUnauthorized {
		t.Fatalf("old token must stay revoked after enable: %d", w.Code)
	}
}

func TestTokenAPI(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")
	bob := env.newUser(t, "bob@example.com")

	for _, body := range []string{`{"name":""}`, `{"name":"   "}`, `{"name":"` + strings.Repeat("x", 65) + `"}`, `{"name":"a","extra":1}`, `{}`} {
		if w := env.request("POST", "/api/v1/tokens", body, alice.token); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(`{"name":"ci"}`))
	r.Header.Set("Authorization", "Bearer "+alice.token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-JSON create: %d", w.Code)
	}

	created := dataOf[struct {
		Token    string         `json:"token"`
		APIToken store.APIToken `json:"api_token"`
	}](t, env.request("POST", "/api/v1/tokens", `{"name":"  ci  "}`, alice.token))
	if !strings.HasPrefix(created.Token, "lil_") || created.APIToken.Name != "ci" || !strings.HasPrefix(created.Token, created.APIToken.TokenPrefix) {
		t.Fatalf("created: %+v", created)
	}
	if w := env.request("GET", "/api/v1/me", "", created.Token); w.Code != http.StatusOK {
		t.Fatalf("new token: %d", w.Code)
	}

	list := env.request("GET", "/api/v1/tokens", "", alice.token)
	if strings.Contains(list.Body.String(), created.Token) {
		t.Fatal("token list exposed plaintext")
	}
	if tokens := dataOf[[]store.APIToken](t, list); len(tokens) != 2 || tokens[0].ID != created.APIToken.ID {
		t.Fatalf("alice tokens: %+v", tokens)
	}
	if tokens := dataOf[[]store.APIToken](t, env.request("GET", "/api/v1/tokens", "", bob.token)); len(tokens) != 1 {
		t.Fatalf("bob sees only his own tokens: %+v", tokens)
	}

	deletePath := "/api/v1/tokens/" + strconv.FormatInt(int64(created.APIToken.ID), 10)
	if w := env.request("DELETE", deletePath, "", bob.token); w.Code != http.StatusNotFound {
		t.Fatalf("revoking another user's token: %d", w.Code)
	}
	if w := env.request("DELETE", "/api/v1/tokens/abc", "", alice.token); w.Code != http.StatusBadRequest {
		t.Fatalf("bad token id: %d", w.Code)
	}
	if w := env.request("DELETE", deletePath, "", alice.token); w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", w.Code)
	}
	if w := env.request("GET", "/api/v1/me", "", created.Token); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d", w.Code)
	}
	if w := env.request("DELETE", deletePath, "", alice.token); w.Code != http.StatusNotFound {
		t.Fatalf("revoking twice: %d", w.Code)
	}
}

func TestAttributionAndAuditAPI(t *testing.T) {
	env := newTestEnv(t)
	alice := env.newUser(t, "alice@example.com")
	bob := env.newUser(t, "bob@example.com")

	created := dataOf[struct {
		ShortCode string `json:"short_code"`
	}](t, env.request("POST", "/api/v1/shorten", `{"url":"https://example.com/a","slug":"promo"}`, alice.token))
	if w := env.request("PUT", "/api/v1/urls/promo", `{"url":"https://example.com/b"}`, bob.token); w.Code != http.StatusNoContent {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}

	urls := dataOf[struct {
		URLs []struct {
			ShortCode string `json:"short_code"`
			CreatedBy *struct {
				Email string `json:"email"`
			} `json:"created_by"`
			UpdatedBy *struct {
				Email string `json:"email"`
			} `json:"updated_by"`
			UpdatedAt *string `json:"updated_at"`
		} `json:"urls"`
	}](t, env.request("GET", "/api/v1/urls", "", alice.token))
	if len(urls.URLs) != 1 || urls.URLs[0].CreatedBy == nil || urls.URLs[0].CreatedBy.Email != "alice@example.com" ||
		urls.URLs[0].UpdatedBy == nil || urls.URLs[0].UpdatedBy.Email != "bob@example.com" || urls.URLs[0].UpdatedAt == nil {
		t.Fatalf("urls: %+v", urls)
	}

	if w := env.request("DELETE", "/api/v1/urls/"+created.ShortCode, "", alice.token); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	env.request("POST", "/api/v1/shorten", `{"url":"https://example.com/other"}`, alice.token)

	type auditPage struct {
		Entries []store.AuditEntry `json:"entries"`
		Page    int64              `json:"page"`
		PerPage int64              `json:"per_page"`
		Count   int64              `json:"count"`
	}
	filtered := dataOf[auditPage](t, env.request("GET", "/api/v1/audit?short_code=promo", "", bob.token))
	if filtered.Count != 3 || len(filtered.Entries) != 3 || filtered.Page != 1 || filtered.PerPage != 10 {
		t.Fatalf("filtered audit: %+v", filtered)
	}
	wantActors := []string{"alice@example.com", "bob@example.com", "alice@example.com"}
	wantActions := []store.AuditAction{store.ActionURLDelete, store.ActionURLUpdate, store.ActionURLCreate}
	for i, entry := range filtered.Entries {
		if entry.Actor.Email != wantActors[i] || entry.Action != wantActions[i] || entry.Target != "promo" || entry.TokenID == nil {
			t.Fatalf("entry %d: %+v", i, entry)
		}
	}
	if all := dataOf[auditPage](t, env.request("GET", "/api/v1/audit?per_page=2&page=2", "", bob.token)); all.Count < 6 || len(all.Entries) != 2 || all.PerPage != 2 {
		t.Fatalf("paged audit: %+v", all)
	}
	for _, query := range []string{"page=0", "per_page=0", "per_page=1001", "page=1000001"} {
		if w := env.request("GET", "/api/v1/audit?"+query, "", bob.token); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
}
