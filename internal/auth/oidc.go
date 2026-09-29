package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mr-karan/lil/internal/store"
	"golang.org/x/oauth2"
)

const (
	sessionState    = "oidc_state"
	sessionNonce    = "oidc_nonce"
	sessionVerifier = "oidc_verifier"
	sessionNext     = "oidc_next"
)

const signedOutPage = `<!doctype html>
<meta charset="utf-8">
<title>Signed out</title>
<p>Signed out. <a href="/auth/login">Sign in again</a></p>
`

func (a *Authenticator) Routes(mux *http.ServeMux) {
	if a.cfg.Mode == ModeDev {
		mux.Handle("GET /auth/login", a.sessions.LoadAndSave(http.HandlerFunc(a.handleDevLogin)))
	} else {
		mux.Handle("GET /auth/login", a.sessions.LoadAndSave(http.HandlerFunc(a.handleLogin)))
		mux.Handle("GET "+callbackPath, a.sessions.LoadAndSave(http.HandlerFunc(a.handleCallback)))
	}
	mux.Handle("POST /auth/logout", a.sessions.LoadAndSave(http.HandlerFunc(a.handleLogout)))
	mux.HandleFunc("GET /auth/signed-out", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(signedOutPage))
	})
}

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Authenticator) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, defaultNext, http.StatusFound)
}

func (a *Authenticator) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	a.sessions.Put(ctx, sessionState, state)
	a.sessions.Put(ctx, sessionNonce, nonce)
	a.sessions.Put(ctx, sessionVerifier, verifier)
	a.sessions.Put(ctx, sessionNext, sanitizeNext(r.URL.Query().Get("next")))
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (a *Authenticator) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.sessions.Destroy(r.Context()); err != nil {
		a.logger.Error("destroy session failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}

// reject logs a fixed reason and answers with it. Callers must never pass
// exchange errors, tokens, codes or query strings as attrs.
func (a *Authenticator) reject(w http.ResponseWriter, status int, reason string, attrs ...any) {
	a.logger.Warn("oidc login rejected", append([]any{"reason", reason}, attrs...)...)
	http.Error(w, reason, status)
}

func (a *Authenticator) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state := a.sessions.PopString(ctx, sessionState)
	nonce := a.sessions.PopString(ctx, sessionNonce)
	verifier := a.sessions.PopString(ctx, sessionVerifier)
	next := sanitizeNext(a.sessions.PopString(ctx, sessionNext))
	query := r.URL.Query()

	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
		a.reject(w, http.StatusBadRequest, "state mismatch")
		return
	}
	if query.Get("error") != "" {
		a.reject(w, http.StatusBadRequest, "identity provider returned an error")
		return
	}

	ctx = oidc.ClientContext(ctx, a.httpClient)
	token, err := a.oauth.Exchange(ctx, query.Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		var retrieveErr *oauth2.RetrieveError
		if errors.As(err, &retrieveErr) {
			a.reject(w, http.StatusBadRequest, "code exchange failed", "http_status", retrieveErr.Response.StatusCode, "error_code", retrieveErr.ErrorCode)
			return
		}
		a.reject(w, http.StatusBadRequest, "code exchange failed")
		return
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		a.reject(w, http.StatusBadRequest, "id token missing")
		return
	}
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		a.reject(w, http.StatusBadRequest, "id token verification failed")
		return
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		a.reject(w, http.StatusBadRequest, "nonce mismatch")
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		HostedDomain  string `json:"hd"`
	}
	if err := idToken.Claims(&claims); err != nil || idToken.Subject == "" {
		a.reject(w, http.StatusBadRequest, "id token claims invalid")
		return
	}
	if !claims.EmailVerified {
		a.reject(w, http.StatusForbidden, "email not verified")
		return
	}
	if !a.cfg.admit(claims.Email, claims.HostedDomain) {
		a.reject(w, http.StatusForbidden, "email not allowed")
		return
	}

	user, err := a.store.UpsertOIDCUser(ctx, a.cfg.IssuerURL, idToken.Subject, claims.Email, claims.Name)
	if errors.Is(err, store.ErrUserDisabled) {
		a.reject(w, http.StatusForbidden, "user disabled")
		return
	}
	if err != nil {
		a.logger.Error("upsert user failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := a.sessions.RenewToken(ctx); err != nil {
		a.logger.Error("renew session token failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	a.sessions.Put(ctx, sessionUserID, int64(user.ID))
	a.sessions.Put(ctx, sessionEpoch, user.SessionEpoch)
	http.Redirect(w, r, next, http.StatusSeeOther)
}
