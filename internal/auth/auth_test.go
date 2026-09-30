package auth

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mr-karan/lil/internal/store"
)

func TestConfigValidation(t *testing.T) {
	valid := Config{
		Mode: ModeOIDC, IssuerURL: "https://idp.example", ClientID: "id", ClientSecret: "secret",
		RedirectURL: "https://lil.example/auth/oidc", AllowedEmails: []string{"a@example.com"},
	}
	for _, tt := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"valid", func(*Config) {}, ""},
		{"loopback http", func(c *Config) {
			c.IssuerURL, c.RedirectURL = "http://127.0.0.1:8080", "http://localhost:7000/auth/oidc"
		}, ""},
		{"emails only", func(c *Config) { c.AllowedDomains = nil }, ""},
		{"emails and domains", func(c *Config) { c.AllowedDomains = []string{"example.com"} }, ""},
		{"domains only", func(c *Config) { c.AllowedDomains, c.AllowedEmails = []string{"example.com"}, nil }, "allowed_emails must list every person"},
		{"no mode", func(c *Config) { c.Mode = "" }, "auth.mode"},
		{"unknown mode", func(c *Config) { c.Mode = "none" }, "auth.mode"},
		{"missing issuer", func(c *Config) { c.IssuerURL = "" }, "issuer_url is required"},
		{"missing client id", func(c *Config) { c.ClientID = "" }, "client_id is required"},
		{"public client without secret", func(c *Config) { c.ClientSecret = "" }, ""},
		{"missing redirect", func(c *Config) { c.RedirectURL = "" }, "redirect_url is required"},
		{"no allowlist", func(c *Config) { c.AllowedEmails = nil }, "allowed_emails must list every person"},
		{"blank allowlist entries", func(c *Config) { c.AllowedEmails = []string{" "} }, "allowed_emails must list every person"},
		{"http issuer", func(c *Config) { c.IssuerURL = "http://idp.example" }, "issuer_url must use https"},
		{"http redirect", func(c *Config) { c.RedirectURL = "http://lil.example/auth/oidc" }, "redirect_url must use https"},
		{"wrong redirect path", func(c *Config) { c.RedirectURL = "https://lil.example/callback" }, "path must be /auth/oidc"},
		{"negative lifetime", func(c *Config) { c.SessionLifetime = -1 }, "session_lifetime"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			cfg.AllowedDomains, cfg.AllowedEmails = normalizeList(cfg.AllowedDomains), normalizeList(cfg.AllowedEmails)
			err := cfg.validate()
			if tt.want == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
	if err := (Config{Mode: ModeDev}).validate(); err == nil || !strings.Contains(err.Error(), "dev_email") {
		t.Fatalf("dev without email: %v", err)
	}
	if err := (Config{Mode: ModeDev, DevEmail: "dev@example.com"}).validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNewFailsWhenDiscoveryFails(t *testing.T) {
	idp := newFakeIdP(t)
	cfg := oidcConfig(idp)
	idp.Close()
	if _, err := New(t.Context(), cfg, newTestStore(t), nopLogger()); err == nil {
		t.Fatal("expected discovery failure")
	}
}

func TestSanitizeNext(t *testing.T) {
	for next, want := range map[string]string{
		"":                            "/admin/",
		"/admin/":                     "/admin/",
		"/admin":                      "/admin",
		"/admin/dashboard?x=1":        "/admin/dashboard?x=1",
		"/admin/activity?a=b&c=d":     "/admin/activity?a=b&c=d",
		"/admin/../promo":             "/admin/",
		"/admin/%2e%2e/promo":         "/admin/",
		"/admin/%2E%2E/promo":         "/admin/",
		"/admin/a%2fb":                "/admin/",
		"/admin/a%5Cb":                "/admin/",
		`/admin/a\b`:                  "/admin/",
		"/admin//x":                   "/admin/",
		"/adminx":                     "/admin/",
		"/promo":                      "/admin/",
		"//evil.example":              "/admin/",
		"//evil.example/admin/":       "/admin/",
		"https://evil.example":        "/admin/",
		"https://evil.example/admin/": "/admin/",
		"javascript:alert(1)":         "/admin/",
		"/admin/\n":                   "/admin/",
	} {
		if got := sanitizeNext(next); got != want {
			t.Errorf("sanitizeNext(%q) = %q, want %q", next, got, want)
		}
	}
}

func TestAdmit(t *testing.T) {
	const idp = "https://idp.example"
	emailsOnly := Config{IssuerURL: idp, AllowedEmails: []string{"guest@other.org"}}
	both := Config{IssuerURL: idp, AllowedDomains: []string{"example.com"}, AllowedEmails: []string{"alice@example.com", "guest@other.org"}}
	googleEmails := emailsOnly
	googleEmails.IssuerURL = googleIssuer
	googleBoth := both
	googleBoth.IssuerURL = googleIssuer
	for _, tt := range []struct {
		name  string
		cfg   Config
		email string
		hd    string
		want  bool
	}{
		{"emails only: listed", emailsOnly, "GUEST@other.org", "", true},
		{"emails only: unlisted", emailsOnly, "bob@other.org", "", false},
		{"emails only: listed email, any domain", emailsOnly, "guest@other.org", "other.org", true},
		{"both: in domain, not listed", both, "bob@example.com", "", false},
		{"both: listed, wrong domain", both, "guest@other.org", "", false},
		{"both: listed, in domain", both, "Alice@Example.com", "", true},
		{"google emails only: needs no hd", googleEmails, "guest@other.org", "", true},
		{"google both: listed, hd matches", googleBoth, "alice@example.com", "example.com", true},
		{"google both: listed, hd absent", googleBoth, "alice@example.com", "", false},
		{"google both: listed, hd mismatched", googleBoth, "alice@example.com", "evil.com", false},
		{"google both: hd matches, not listed", googleBoth, "bob@example.com", "example.com", false},
		{"empty allowlist admits nobody", Config{}, "alice@example.com", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.admit(tt.email, tt.hd); got != tt.want {
				t.Fatalf("admit = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDevMode(t *testing.T) {
	s := newTestStore(t)
	fx := newFixtureWith(t, nil, s, Config{Mode: ModeDev, DevEmail: "dev@example.com"})
	if w := fx.do("GET", "/api/x", nil, nil); w.Code != 200 || w.Body.String() != "dev@example.com" {
		t.Fatalf("api: %d %s", w.Code, w.Body.String())
	}
	if w := fx.do("GET", "/admin/", nil, nil); w.Code != 200 {
		t.Fatalf("admin: %d", w.Code)
	}
	if w := fx.do("GET", "/auth/login", nil, nil); w.Code != http.StatusFound || w.Header().Get("Location") != "/admin/" {
		t.Fatalf("login: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := fx.do("POST", "/auth/logout", nil, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := fx.do("GET", "/auth/signed-out", nil, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Signed out") {
		t.Fatalf("signed-out: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(fx.logs.String(), "auth dev mode: every request is signed in as dev@example.com") {
		t.Fatalf("missing dev warning: %s", fx.logs.String())
	}
	if w := fx.do("GET", "/api/x", nil, map[string]string{"Authorization": "Bearer lil_bogus"}); w.Code != 401 {
		t.Fatalf("bad bearer in dev mode: %d", w.Code)
	}

	dev := fx.userByEmail("dev@example.com")
	if err := s.SetUserDisabled(t.Context(), store.Actor{UserID: dev.ID}, dev.ID, true); err != nil {
		t.Fatal(err)
	}
	if w := fx.do("GET", "/api/x", nil, nil); w.Code != 401 {
		t.Fatalf("disabled dev user: %d", w.Code)
	}
}

func TestUnauthenticatedResponses(t *testing.T) {
	fx := newFixture(t, nil)
	w := fx.do("GET", "/api/x", nil, nil)
	if w.Code != 401 || w.Body.String() != `{"status":"error","message":"authentication required"}` || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("api: %d %q %s", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	w = fx.do("GET", "/admin/", nil, nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/auth/login?next=%2Fadmin%2F" {
		t.Fatalf("admin: %d %s", w.Code, w.Header().Get("Location"))
	}
	w = fx.do("GET", "/admin/activity?short_code=abc", nil, nil)
	if w.Header().Get("Location") != "/auth/login?next=%2Fadmin%2Factivity%3Fshort_code%3Dabc" {
		t.Fatalf("deep link: %s", w.Header().Get("Location"))
	}
	for _, header := range []string{"Basic dXNlcjpwYXNz", "Bearer", "Bearer ", "Token abc"} {
		if w := fx.do("GET", "/api/x", nil, map[string]string{"Authorization": header}); w.Code != 401 {
			t.Fatalf("Authorization %q: %d", header, w.Code)
		}
	}
}
