package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/mr-karan/lil/internal/store"
)

const testClientID = "client-id"

// fakeIdP is the only mock in this package: an external OIDC provider.
type fakeIdP struct {
	*httptest.Server
	key *rsa.PrivateKey

	mu          sync.Mutex
	nonce       string
	overrides   map[string]any
	errorStatus int
	errorBody   string
	lastToken   tokenRequest
}

type tokenRequest struct {
	form          url.Values
	authorization string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, overrides: map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.URL,
			"authorization_endpoint":                f.URL + "/authorize",
			"token_endpoint":                        f.URL + "/token",
			"jwks_uri":                              f.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", f.serveToken)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeIdP) serveToken(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errorStatus != 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.errorStatus)
		io.WriteString(w, f.errorBody)
		return
	}
	if err := r.ParseForm(); err != nil || r.PostForm.Get("code") == "" || r.PostForm.Get("code_verifier") == "" {
		http.Error(w, "missing code or code_verifier", http.StatusBadRequest)
		return
	}
	f.lastToken = tokenRequest{form: r.PostForm, authorization: r.Header.Get("Authorization")}
	claims := map[string]any{
		"iss": f.URL, "aud": testClientID, "sub": "sub-1", "nonce": f.nonce,
		"email": "alice@example.com", "email_verified": true, "name": "Alice",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}
	for k, v := range f.overrides {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idToken, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idToken})
}

func (f *fakeIdP) set(nonce string, overrides map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nonce, f.overrides = nonce, overrides
}

func (f *fakeIdP) tokenRequest() tokenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastToken
}

func (f *fakeIdP) failToken(status int, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errorStatus, f.errorBody = status, body
}

type fixture struct {
	t       *testing.T
	idp     *fakeIdP
	store   *store.Store
	authn   *Authenticator
	handler http.Handler
	logs    *bytes.Buffer
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(store.Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func oidcConfig(idp *fakeIdP) Config {
	return Config{
		Mode:           ModeOIDC,
		IssuerURL:      idp.URL,
		ClientID:       testClientID,
		ClientSecret:   "client-secret",
		RedirectURL:    "http://127.0.0.1/auth/oidc",
		AllowedDomains: []string{"Example.com"},
		AllowedEmails:  []string{"Alice@Example.com", "bob@example.com"},
	}
}

func newFixture(t *testing.T, mutate func(*Config)) *fixture {
	t.Helper()
	idp := newFakeIdP(t)
	s := newTestStore(t)
	cfg := oidcConfig(idp)
	if mutate != nil {
		mutate(&cfg)
	}
	return newFixtureWith(t, idp, s, cfg)
}

func newFixtureWith(t *testing.T, idp *fakeIdP, s *store.Store, cfg Config) *fixture {
	t.Helper()
	logs := &bytes.Buffer{}
	authn, err := New(t.Context(), cfg, s, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	authn.Routes(mux)
	mux.Handle("/api/", authn.RequireAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := UserFrom(r.Context())
		io.WriteString(w, user.Email)
	})))
	mux.Handle("/admin/", authn.RequireBrowser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "admin")
	})))
	return &fixture{t: t, idp: idp, store: s, authn: authn, handler: mux, logs: logs}
}

func (fx *fixture) do(method, target string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	fx.t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	fx.handler.ServeHTTP(w, r)
	return w
}

type loginStart struct {
	cookies []*http.Cookie
	state   string
	nonce   string
	authURL *url.URL
}

func (fx *fixture) startLogin(next string) loginStart {
	fx.t.Helper()
	w := fx.do("GET", "/auth/login?next="+url.QueryEscape(next), nil, nil)
	if w.Code != http.StatusFound {
		fx.t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		fx.t.Fatal(err)
	}
	return loginStart{cookies: w.Result().Cookies(), state: loc.Query().Get("state"), nonce: loc.Query().Get("nonce"), authURL: loc}
}

// finishLogin runs the callback. The fake IdP echoes the nonce unless overrides replace it.
func (fx *fixture) finishLogin(start loginStart, overrides map[string]any) *httptest.ResponseRecorder {
	fx.t.Helper()
	fx.idp.set(start.nonce, overrides)
	return fx.do("GET", "/auth/oidc?code=abc&state="+url.QueryEscape(start.state), start.cookies, nil)
}

func (fx *fixture) login(email string) []*http.Cookie {
	fx.t.Helper()
	w := fx.finishLogin(fx.startLogin("/admin/"), map[string]any{"email": email})
	if w.Code != http.StatusSeeOther {
		fx.t.Fatalf("login callback: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()
}

func (fx *fixture) userByEmail(email string) store.User {
	fx.t.Helper()
	users, err := fx.store.ListUsers(fx.t.Context())
	if err != nil {
		fx.t.Fatal(err)
	}
	for _, u := range users {
		if u.Email == email {
			return u
		}
	}
	fx.t.Fatalf("user %s not found", email)
	return store.User{}
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
