package auth

import (
	"net/http"
	"testing"

	"github.com/mr-karan/lil/internal/store"
)

func (fx *fixture) token(email string) (string, store.User) {
	fx.t.Helper()
	fx.login(email)
	user := fx.userByEmail(email)
	plaintext, _, err := fx.store.CreateAPIToken(fx.t.Context(), store.Actor{UserID: user.ID}, "ci")
	if err != nil {
		fx.t.Fatal(err)
	}
	return plaintext, user
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestBearerToken(t *testing.T) {
	fx := newFixture(t, nil)
	token, _ := fx.token("alice@example.com")
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 200 || w.Body.String() != "alice@example.com" {
		t.Fatalf("valid token: %d %s", w.Code, w.Body.String())
	}
	if w := fx.do("GET", "/api/x", nil, map[string]string{"Authorization": "bearer " + token}); w.Code != 200 {
		t.Fatalf("scheme is case-insensitive: %d", w.Code)
	}
}

func TestInvalidBearerNeverFallsBackToSession(t *testing.T) {
	fx := newFixture(t, nil)
	cookies := fx.login("alice@example.com")
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 200 {
		t.Fatalf("session alone must work: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", cookies, bearer("lil_invalid")); w.Code != 401 {
		t.Fatalf("invalid bearer with valid session: %d", w.Code)
	}
	if w := fx.do("GET", "/admin/", cookies, bearer("lil_invalid")); w.Code != http.StatusFound {
		t.Fatalf("invalid bearer on browser route: %d", w.Code)
	}
}

func TestDisableThenEnableDoesNotRestoreCredentials(t *testing.T) {
	fx := newFixture(t, nil)
	token, alice := fx.token("alice@example.com")
	cookies := fx.login("alice@example.com")
	admin := store.Actor{UserID: alice.ID}

	if err := fx.store.SetUserDisabled(t.Context(), admin, alice.ID, true); err != nil {
		t.Fatal(err)
	}
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 401 {
		t.Fatalf("token after disable: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("session after disable: %d", w.Code)
	}

	if err := fx.store.SetUserDisabled(t.Context(), admin, alice.ID, false); err != nil {
		t.Fatal(err)
	}
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 401 {
		t.Fatalf("old token after enable: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("old session after enable: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", fx.login("alice@example.com"), nil); w.Code != 200 {
		t.Fatalf("fresh login after enable: %d", w.Code)
	}
}

func TestAllowlistIsRecheckedOnEveryRequest(t *testing.T) {
	fx := newFixture(t, nil)
	token, _ := fx.token("alice@example.com")
	cookies := fx.login("alice@example.com")
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 200 {
		t.Fatalf("token before change: %d", w.Code)
	}

	fx.authn.cfg.AllowedEmails = []string{"carol@example.com"}
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 401 {
		t.Fatalf("token after email removed, domain still matches: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("session after email removed, domain still matches: %d", w.Code)
	}

	fx.authn.cfg.AllowedEmails = []string{"alice@example.com", "bob@example.com"}
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 200 {
		t.Fatalf("email restored: %d", w.Code)
	}

	fx.authn.cfg.AllowedDomains = []string{"other.org"}
	if w := fx.do("GET", "/api/x", nil, bearer(token)); w.Code != 401 {
		t.Fatalf("token after domain changed, email still listed: %d", w.Code)
	}
	if w := fx.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("session after domain changed, email still listed: %d", w.Code)
	}
}

func TestDevCredentialsDoNotSurviveSwitchToOIDC(t *testing.T) {
	idp := newFakeIdP(t)
	s := newTestStore(t)
	dev := newFixtureWith(t, idp, s, Config{Mode: ModeDev, DevEmail: "alice@example.com"})
	devUser := dev.userByEmail("alice@example.com")
	token, _, err := s.CreateAPIToken(t.Context(), store.Actor{UserID: devUser.ID}, "dev token")
	if err != nil {
		t.Fatal(err)
	}
	if w := dev.do("GET", "/api/x", nil, bearer(token)); w.Code != 200 {
		t.Fatalf("token in dev mode: %d", w.Code)
	}

	prod := newFixtureWith(t, idp, s, oidcConfig(idp))
	if w := prod.do("GET", "/api/x", nil, bearer(token)); w.Code != 401 {
		t.Fatalf("dev token in oidc mode: %d", w.Code)
	}
	if w := prod.do("GET", "/api/x", nil, nil); w.Code != 401 {
		t.Fatalf("dev user in oidc mode: %d", w.Code)
	}
}

func TestSessionForUnknownUserIsRejected(t *testing.T) {
	fx := newFixture(t, nil)
	cookies := fx.login("alice@example.com")
	other := newFixtureWith(t, fx.idp, newTestStore(t), oidcConfig(fx.idp))
	if w := other.do("GET", "/api/x", cookies, nil); w.Code != 401 {
		t.Fatalf("session cookie unknown to this database: %d", w.Code)
	}
}
