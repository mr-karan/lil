package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mr-karan/lil/internal/store"
)

func TestAuthorizeRedirect(t *testing.T) {
	fx := newFixture(t, nil)
	start := fx.startLogin("/admin/")
	q := start.authURL.Query()
	if start.authURL.Host != strings.TrimPrefix(fx.idp.URL, "http://") || start.authURL.Path != "/authorize" {
		t.Fatalf("authorize URL: %s", start.authURL)
	}
	for key, want := range map[string]string{
		"scope": "openid email profile", "code_challenge_method": "S256", "response_type": "code",
		"client_id": testClientID, "redirect_uri": "http://127.0.0.1/auth/oidc",
	} {
		if q.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, q.Get(key), want)
		}
	}
	for _, key := range []string{"state", "nonce", "code_challenge"} {
		if len(q.Get(key)) < 32 {
			t.Errorf("%s too short: %q", key, q.Get(key))
		}
	}
	if len(start.cookies) != 1 || start.cookies[0].Name != "lil_session" || !start.cookies[0].HttpOnly || start.cookies[0].SameSite != http.SameSiteLaxMode || start.cookies[0].Path != "/" {
		t.Fatalf("cookie: %+v", start.cookies)
	}
	if start.cookies[0].Secure {
		t.Fatal("cookie must not be Secure for an http redirect_url")
	}
}

func TestCallbackPublicClientSendsNoSecret(t *testing.T) {
	fx := newFixture(t, func(c *Config) { c.ClientSecret = "" })
	w := fx.finishLogin(fx.startLogin("/admin/"), nil)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	req := fx.idp.tokenRequest()
	if req.form.Get("client_id") != testClientID || req.form.Get("code_verifier") == "" {
		t.Fatalf("token form: %v", req.form)
	}
	if req.form.Has("client_secret") || req.authorization != "" {
		t.Fatalf("public client sent a secret: form=%v authorization=%q", req.form, req.authorization)
	}
}

func TestCallbackConfidentialClientSendsSecret(t *testing.T) {
	fx := newFixture(t, nil)
	if w := fx.finishLogin(fx.startLogin("/admin/"), nil); w.Code != http.StatusSeeOther {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	req := fx.idp.tokenRequest()
	if req.form.Get("client_secret") != "client-secret" && req.authorization == "" {
		t.Fatalf("confidential client sent no secret: form=%v", req.form)
	}
}

func TestCallbackAllowlistIsAnd(t *testing.T) {
	for _, tt := range []struct {
		name      string
		mutate    func(*Config)
		overrides map[string]any
		status    int
	}{
		{"emails only, listed", func(c *Config) { c.AllowedDomains = nil }, nil, http.StatusSeeOther},
		{"emails only, listed in any domain", func(c *Config) { c.AllowedDomains, c.AllowedEmails = nil, []string{"guest@other.org"} }, map[string]any{"email": "guest@other.org"}, http.StatusSeeOther},
		{"emails only, unlisted", func(c *Config) { c.AllowedDomains, c.AllowedEmails = nil, []string{"bob@example.com"} }, nil, http.StatusForbidden},
		{"both, listed in domain", nil, nil, http.StatusSeeOther},
		{"both, in domain not listed", func(c *Config) { c.AllowedEmails = []string{"bob@example.com"} }, nil, http.StatusForbidden},
		{"both, listed in wrong domain", func(c *Config) { c.AllowedEmails = []string{"guest@other.org"} }, map[string]any{"email": "guest@other.org"}, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t, tt.mutate)
			w := fx.finishLogin(fx.startLogin("/admin/"), tt.overrides)
			if w.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestCallbackHappyPath(t *testing.T) {
	fx := newFixture(t, nil)
	start := fx.startLogin("/admin/dashboard?x=1")
	w := fx.finishLogin(start, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/admin/dashboard?x=1" {
		t.Fatalf("callback: %d %s %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Value == start.cookies[0].Value {
		t.Fatal("session token was not renewed at login")
	}
	user := fx.userByEmail("alice@example.com")
	if user.Issuer != fx.idp.URL || user.LastLoginAt == nil {
		t.Fatalf("user: %+v", user)
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 200 || w.Body.String() != "alice@example.com" {
		t.Fatalf("session request: %d %s", w.Code, w.Body.String())
	}
	if w := fx.do("GET", "/admin/", cookies, nil); w.Code != 200 {
		t.Fatalf("browser request: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", start.cookies, nil); w.Code != 401 {
		t.Fatalf("pre-login session token must not authenticate: %d", w.Code)
	}

	w = fx.do("POST", "/auth/logout", cookies, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("session after logout: %d", w.Code)
	}
}

func TestCallbackSanitizesNext(t *testing.T) {
	for _, next := range []string{"https://evil.example", "//evil.example", "/admin/../promo"} {
		fx := newFixture(t, nil)
		w := fx.finishLogin(fx.startLogin(next), nil)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/admin/" {
			t.Fatalf("next %q: %d %s", next, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestCallbackRejections(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-2 * time.Hour).Unix()
	for _, tt := range []struct {
		name      string
		overrides map[string]any
		status    int
		reason    string
	}{
		{"nonce mismatch", map[string]any{"nonce": "wrong"}, 400, "nonce mismatch"},
		{"wrong audience", map[string]any{"aud": "someone-else"}, 400, "id token verification failed"},
		{"wrong issuer", map[string]any{"iss": "https://evil.example"}, 400, "id token verification failed"},
		{"expired", map[string]any{"exp": past}, 400, "id token verification failed"},
		{"missing subject", map[string]any{"sub": nil}, 400, "id token claims invalid"},
		{"email not verified", map[string]any{"email_verified": false}, 403, "email not verified"},
		{"email_verified absent", map[string]any{"email_verified": nil}, 403, "email not verified"},
		{"disallowed domain", map[string]any{"email": "mallory@evil.com"}, 403, "email not allowed"},
		{"suffix domain", map[string]any{"email": "mallory@notexample.com", "exp": future}, 403, "email not allowed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t, nil)
			w := fx.finishLogin(fx.startLogin("/admin/"), tt.overrides)
			if w.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
			if !strings.Contains(fx.logs.String(), "reason=\""+tt.reason+"\"") {
				t.Fatalf("log missing reason %q: %s", tt.reason, fx.logs.String())
			}
			users, err := fx.store.ListUsers(t.Context())
			if err != nil || len(users) != 0 {
				t.Fatalf("rejected login must not create a user: %v %v", users, err)
			}
		})
	}
}

func TestCallbackStateAndProtocolErrors(t *testing.T) {
	fx := newFixture(t, nil)
	start := fx.startLogin("/admin/")
	fx.idp.set(start.nonce, nil)
	if w := fx.do("GET", "/auth/oidc?code=abc&state=wrong", start.cookies, nil); w.Code != 400 {
		t.Fatalf("state mismatch: %d", w.Code)
	}
	if w := fx.do("GET", "/auth/oidc?code=abc&state="+start.state, start.cookies, nil); w.Code != 400 {
		t.Fatalf("state must be single-use, replay got %d", w.Code)
	}
	if w := fx.do("GET", "/auth/oidc?code=abc&state="+start.state, nil, nil); w.Code != 400 {
		t.Fatalf("no session: %d", w.Code)
	}

	start = fx.startLogin("/admin/")
	if w := fx.do("GET", "/auth/oidc?error=access_denied&state="+start.state, start.cookies, nil); w.Code != 400 {
		t.Fatalf("idp error: %d", w.Code)
	}
	if w := fx.do("GET", "/auth/oidc?code=abc&state="+start.state, start.cookies, nil); w.Code != 400 {
		t.Fatalf("state must be removed after an idp error: %d", w.Code)
	}
}

func TestCallbackDisabledUser(t *testing.T) {
	fx := newFixture(t, nil)
	fx.login("alice@example.com")
	alice := fx.userByEmail("alice@example.com")
	if err := fx.store.SetUserDisabled(t.Context(), store.Actor{UserID: alice.ID}, alice.ID, true); err != nil {
		t.Fatal(err)
	}
	w := fx.finishLogin(fx.startLogin("/admin/"), nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("disabled user login: %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Value != "" && fx.do("GET", "/api/x", []*http.Cookie{c}, nil).Code == 200 {
			t.Fatal("disabled login produced a working session")
		}
	}
}

func TestCallbackGoogleHostedDomain(t *testing.T) {
	for _, tt := range []struct {
		name      string
		overrides map[string]any
		status    int
	}{
		{"listed email, hd matches", map[string]any{"hd": "example.com"}, http.StatusSeeOther},
		{"listed email, hd absent", nil, http.StatusForbidden},
		{"listed email, hd mismatched", map[string]any{"hd": "evil.com"}, http.StatusForbidden},
		{"hd matches, email not listed", map[string]any{"email": "carol@example.com", "hd": "example.com"}, http.StatusForbidden},
		{"listed email, wrong domain", map[string]any{"email": "guest@other.org", "hd": "other.org"}, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t, func(c *Config) { c.AllowedEmails = []string{"alice@example.com", "bob@example.com", "guest@other.org"} })
			fx.authn.cfg.IssuerURL = googleIssuer
			w := fx.finishLogin(fx.startLogin("/admin/"), tt.overrides)
			if w.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestCallbackDoesNotLogExchangeDetails(t *testing.T) {
	const sentinel = "SENTINEL-SECRET-8f3a"
	for _, body := range []string{
		`{"error":"invalid_grant","error_description":"` + sentinel + `"}`,
		`not json ` + sentinel,
	} {
		fx := newFixture(t, nil)
		start := fx.startLogin("/admin/")
		fx.idp.failToken(http.StatusBadRequest, body)
		w := fx.do("GET", "/auth/oidc?code=code-"+sentinel+"&state="+start.state, start.cookies, nil)
		if w.Code != 400 {
			t.Fatalf("status %d", w.Code)
		}
		logs := fx.logs.String()
		if strings.Contains(logs, sentinel) || strings.Contains(w.Body.String(), sentinel) {
			t.Fatalf("sentinel leaked: %s %s", logs, w.Body.String())
		}
		if !strings.Contains(logs, `reason="code exchange failed"`) || !strings.Contains(logs, "http_status=400") {
			t.Fatalf("missing exchange log fields: %s", logs)
		}
		if strings.HasPrefix(body, "{") && !strings.Contains(logs, "error_code=invalid_grant") {
			t.Fatalf("missing RFC 6749 error code: %s", logs)
		}
	}
}

func newDebugFixture(t *testing.T, level slog.Level) *fixture {
	t.Helper()
	fx := newFixture(t, nil)
	fx.logs.Reset()
	fx.authn.logger = slog.New(slog.NewJSONHandler(fx.logs, &slog.HandlerOptions{Level: level}))
	return fx
}

func TestCallbackDebugLogsClaimShape(t *testing.T) {
	fx := newDebugFixture(t, slog.LevelDebug)
	start := fx.startLogin("/admin/")
	if w := fx.finishLogin(start, nil); w.Code != http.StatusSeeOther {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}

	lines := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(fx.logs.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		lines[rec["msg"].(string)] = rec
	}
	idLine, infoLine := lines["oidc id token claims"], lines["oidc userinfo claims"]
	if idLine == nil || infoLine == nil {
		t.Fatalf("missing claim log lines: %s", fx.logs.String())
	}
	if got, want := fmt.Sprint(idLine["claims"]), "[aud email email_verified exp iat iss name nonce sub]"; got != want {
		t.Errorf("id token claims = %s, want %s", got, want)
	}
	if got, want := fmt.Sprint(infoLine["claims"]), "[email email_verified name sub]"; got != want {
		t.Errorf("userinfo claims = %s, want %s", got, want)
	}
	for name, rec := range lines {
		if rec["email_domain"] != "example.com" || rec["email_verified"] != true || rec["email_verified_type"] != "bool" || rec["has_name"] != true || rec["has_hd"] != false {
			t.Errorf("%s attrs: %v", name, rec)
		}
	}
	if infoLine["sub_matches_id_token"] != true {
		t.Errorf("sub_matches_id_token = %v", infoLine["sub_matches_id_token"])
	}

	logs := fx.logs.String()
	for _, secret := range []string{"alice@example.com", "Alice", testAccessToken, "code=abc", "abc&", "sub-1", start.state, start.nonce} {
		if strings.Contains(logs, secret) {
			t.Errorf("logs leak %q: %s", secret, logs)
		}
	}
}

func TestCallbackInfoLevelSkipsUserInfo(t *testing.T) {
	fx := newDebugFixture(t, slog.LevelInfo)
	if w := fx.finishLogin(fx.startLogin("/admin/"), nil); w.Code != http.StatusSeeOther {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	if n := fx.idp.userInfoCalls(); n != 0 {
		t.Fatalf("userinfo called %d times at info level", n)
	}
}
