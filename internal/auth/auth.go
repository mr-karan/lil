package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mr-karan/lil/internal/store"
	"golang.org/x/oauth2"
)

type Mode string

const (
	ModeOIDC Mode = "oidc"
	ModeDev  Mode = "dev"
)

const (
	googleIssuer           = "https://accounts.google.com"
	callbackPath           = "/auth/oidc"
	defaultSessionLifetime = 168 * time.Hour
	discoveryTimeout       = 15 * time.Second
	idpClientTimeout       = 10 * time.Second

	sessionUserID = "user_id"
	sessionEpoch  = "session_epoch"
)

type Config struct {
	Mode            Mode
	DevEmail        string
	SessionLifetime time.Duration
	IssuerURL       string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	AllowedDomains  []string
	AllowedEmails   []string
}

func (c Config) validate() error {
	if c.SessionLifetime < 0 {
		return errors.New("auth.session_lifetime must not be negative")
	}
	switch c.Mode {
	case ModeDev:
		if c.DevEmail == "" {
			return errors.New("auth.dev_email is required when auth.mode is dev")
		}
		return nil
	case ModeOIDC:
	default:
		return fmt.Errorf("auth.mode must be %q or %q, got %q", ModeOIDC, ModeDev, c.Mode)
	}
	for name, value := range map[string]string{
		"auth.oidc.issuer_url":   c.IssuerURL,
		"auth.oidc.client_id":    c.ClientID,
		"auth.oidc.redirect_url": c.RedirectURL,
	} {
		if value == "" {
			return fmt.Errorf("%s is required when auth.mode is oidc", name)
		}
	}
	if len(c.AllowedEmails) == 0 {
		return errors.New("auth.oidc.allowed_emails must list every person allowed to sign in")
	}
	if _, err := secureURL("auth.oidc.issuer_url", c.IssuerURL); err != nil {
		return err
	}
	redirect, err := secureURL("auth.oidc.redirect_url", c.RedirectURL)
	if err != nil {
		return err
	}
	if redirect.Path != callbackPath {
		return fmt.Errorf("auth.oidc.redirect_url path must be %s", callbackPath)
	}
	return nil
}

func secureURL(name, raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%s is not a valid absolute URL", name)
	}
	if u.Scheme == "https" {
		return u, nil
	}
	host := u.Hostname()
	if u.Scheme == "http" && (host == "localhost" || net.ParseIP(host).IsLoopback()) {
		return u, nil
	}
	return nil, fmt.Errorf("%s must use https (http is allowed only for localhost)", name)
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func emailDomain(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return email[i+1:]
}

// allowed applies the allowlist: the lowercased email must be in AllowedEmails,
// and when AllowedDomains is set, domain must also be in it.
func (c Config) allowed(email, domain string) bool {
	if !slices.Contains(c.AllowedEmails, strings.ToLower(email)) {
		return false
	}
	return len(c.AllowedDomains) == 0 || slices.Contains(c.AllowedDomains, strings.ToLower(domain))
}

func (c Config) emailAllowed(email string) bool {
	return c.allowed(email, emailDomain(email))
}

// admit decides whether a verified login may sign in. Google issues consumer
// accounts with verified company addresses, so on Google the domain check uses
// the signed hd claim instead of the email domain.
func (c Config) admit(email, hd string) bool {
	if c.IssuerURL == googleIssuer {
		return c.allowed(email, hd)
	}
	return c.allowed(email, emailDomain(email))
}

type Authenticator struct {
	cfg        Config
	store      *store.Store
	logger     *slog.Logger
	sessions   *scs.SessionManager
	devUserID  store.UserID
	oauth      *oauth2.Config
	provider   *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	httpClient *http.Client
}

func New(ctx context.Context, cfg Config, st *store.Store, logger *slog.Logger) (*Authenticator, error) {
	cfg.AllowedDomains = normalizeList(cfg.AllowedDomains)
	cfg.AllowedEmails = normalizeList(cfg.AllowedEmails)
	if cfg.SessionLifetime == 0 {
		cfg.SessionLifetime = defaultSessionLifetime
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	sessions := scs.New()
	sessions.Store = st.SessionStore()
	sessions.Lifetime = cfg.SessionLifetime
	sessions.Cookie.Name = "lil_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = strings.HasPrefix(cfg.RedirectURL, "https://")
	sessions.Cookie.Path = "/"

	a := &Authenticator{
		cfg:        cfg,
		store:      st,
		logger:     logger,
		sessions:   sessions,
		httpClient: &http.Client{Timeout: idpClientTimeout},
	}

	if cfg.Mode == ModeDev {
		user, err := st.EnsureDevUser(ctx, cfg.DevEmail)
		if err != nil {
			return nil, fmt.Errorf("ensure dev user: %w", err)
		}
		a.devUserID = user.ID
		logger.Warn(fmt.Sprintf("auth dev mode: every request is signed in as %s", cfg.DevEmail))
		return a, nil
	}

	discoveryCtx, cancel := context.WithTimeout(oidc.ClientContext(ctx, a.httpClient), discoveryTimeout)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryCtx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	a.provider = provider
	endpoint := provider.Endpoint()
	if cfg.ClientSecret == "" {
		endpoint.AuthStyle = oauth2.AuthStyleInParams
	}
	a.oauth = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     endpoint,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
	a.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return a, nil
}

var errUnauthenticated = errors.New("unauthenticated")

// identify resolves the caller. A bearer header means bearer only: an invalid
// token never falls back to the session.
func (a *Authenticator) identify(r *http.Request) (store.Actor, store.User, error) {
	ctx := r.Context()
	if token, ok := bearerToken(r); ok {
		if token == "" {
			return store.Actor{}, store.User{}, errUnauthenticated
		}
		actor, user, err := a.store.AuthenticateAPIToken(ctx, token)
		if errors.Is(err, store.ErrUnauthorized) {
			return store.Actor{}, store.User{}, errUnauthenticated
		}
		if err != nil {
			return store.Actor{}, store.User{}, err
		}
		if !a.permitted(user) {
			return store.Actor{}, store.User{}, errUnauthenticated
		}
		return actor, user, nil
	}

	if a.cfg.Mode == ModeDev {
		return a.identifyDevUser(ctx)
	}

	id := a.sessions.GetInt64(ctx, sessionUserID)
	if id == 0 {
		return store.Actor{}, store.User{}, errUnauthenticated
	}
	user, err := a.store.GetUser(ctx, store.UserID(id))
	if err != nil && !errors.Is(err, store.ErrUserNotExist) {
		return store.Actor{}, store.User{}, err
	}
	if err != nil || user.DisabledAt != nil || user.SessionEpoch != a.sessions.GetInt64(ctx, sessionEpoch) || !a.permitted(user) {
		if err := a.sessions.Destroy(ctx); err != nil {
			return store.Actor{}, store.User{}, err
		}
		return store.Actor{}, store.User{}, errUnauthenticated
	}
	return store.Actor{UserID: user.ID}, user, nil
}

// bearerToken reports whether the request carries an API token. Other
// Authorization schemes are ignored: browsers keep sending cached Basic
// credentials to a site long after it stopped asking for them.
func bearerToken(r *http.Request) (string, bool) {
	for _, value := range r.Header.Values("Authorization") {
		if scheme, token, _ := strings.Cut(value, " "); strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token), true
		}
	}
	return "", false
}

func (a *Authenticator) identifyDevUser(ctx context.Context) (store.Actor, store.User, error) {
	user, err := a.store.GetUser(ctx, a.devUserID)
	if err != nil {
		return store.Actor{}, store.User{}, err
	}
	if user.DisabledAt != nil {
		return store.Actor{}, store.User{}, errUnauthenticated
	}
	return store.Actor{UserID: user.ID}, user, nil
}

// permitted enforces issuer pinning and the current allowlist on every request.
func (a *Authenticator) permitted(user store.User) bool {
	if a.cfg.Mode == ModeDev {
		return true
	}
	return user.Issuer == a.cfg.IssuerURL && a.cfg.emailAllowed(user.Email)
}

func (a *Authenticator) require(next http.Handler, unauthenticated http.HandlerFunc) http.Handler {
	return a.sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, user, err := a.identify(r)
		if errors.Is(err, errUnauthenticated) {
			unauthenticated(w, r)
			return
		}
		if err != nil {
			a.logger.Error("resolve identity failed", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), actor, user)))
	}))
}

// RequireAPI answers unauthenticated callers with a 401 JSON envelope.
func (a *Authenticator) RequireAPI(next http.Handler) http.Handler {
	return a.require(next, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("WWW-Authenticate", "Bearer")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":"error","message":"authentication required"}`))
	})
}

// RequireBrowser sends unauthenticated callers to the login page.
func (a *Authenticator) RequireBrowser(next http.Handler) http.Handler {
	return a.require(next, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
	})
}
