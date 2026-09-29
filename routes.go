package main

import (
	"mime"
	"net/http"

	"github.com/VictoriaMetrics/metrics"
	"github.com/mr-karan/lil/internal/auth"
	appmetrics "github.com/mr-karan/lil/internal/metrics"
)

func (app *App) routes(authn *auth.Authenticator) http.Handler {
	mux := http.NewServeMux()
	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1", app.handleIndex)
	api.Handle("POST /api/v1/shorten", requireJSON(http.HandlerFunc(app.handleShortenURL)))
	api.HandleFunc("GET /api/v1/urls", app.handleGetURLs)
	api.Handle("PUT /api/v1/urls/{shortCode}", requireJSON(http.HandlerFunc(app.handleUpdateURL)))
	api.HandleFunc("DELETE /api/v1/urls/{shortCode}", app.handleDeleteURL)
	api.HandleFunc("GET /api/v1/me", app.handleMe)
	api.HandleFunc("GET /api/v1/users", app.handleListUsers)
	api.HandleFunc("POST /api/v1/users/{id}/disable", app.handleDisableUser)
	api.HandleFunc("POST /api/v1/users/{id}/enable", app.handleEnableUser)
	api.HandleFunc("GET /api/v1/tokens", app.handleListTokens)
	api.Handle("POST /api/v1/tokens", requireJSON(http.HandlerFunc(app.handleCreateToken)))
	api.HandleFunc("DELETE /api/v1/tokens/{id}", app.handleRevokeToken)
	api.HandleFunc("GET /api/v1/audit", app.handleListAudit)
	api.HandleFunc("GET /api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		count, err := app.store.Count(r.Context())
		if err != nil {
			app.sendErrorResponse(w, "Could not read stored URL count", http.StatusServiceUnavailable, nil)
			return
		}
		appmetrics.URLsStoredGauge.Set(float64(count))
		metrics.WritePrometheus(w, true)
	})
	authn.Routes(mux)
	mux.Handle("/api/", authn.RequireAPI(api))
	mux.Handle("GET /admin/", authn.RequireBrowser(getAdminUI()))
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /api/v1/health", app.handleHealthCheck)
	mux.HandleFunc("GET /{shortCode}", app.handleRedirect)
	protected := http.NewCrossOriginProtection().Handler(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		protected.ServeHTTP(w, r)
	})
}

func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}
