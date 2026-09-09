package main

import (
	"mime"
	"net/http"

	"github.com/VictoriaMetrics/metrics"
	appmetrics "github.com/mr-karan/lil/internal/metrics"
	"github.com/mr-karan/lil/internal/middleware"
)

func (app *App) routes(username, password string) http.Handler {
	mux := http.NewServeMux()
	admin := http.NewServeMux()
	admin.HandleFunc("GET /api/v1", app.handleIndex)
	admin.Handle("POST /api/v1/shorten", requireJSON(http.HandlerFunc(app.handleShortenURL)))
	admin.HandleFunc("GET /api/v1/urls", app.handleGetURLs)
	admin.Handle("PUT /api/v1/urls/{shortCode}", requireJSON(http.HandlerFunc(app.handleUpdateURL)))
	admin.HandleFunc("DELETE /api/v1/urls/{shortCode}", app.handleDeleteURL)
	admin.HandleFunc("GET /api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		count, err := app.store.Count(r.Context())
		if err != nil {
			app.sendErrorResponse(w, "Could not read stored URL count", http.StatusServiceUnavailable, nil)
			return
		}
		appmetrics.URLsStoredGauge.Set(float64(count))
		metrics.WritePrometheus(w, true)
	})
	admin.Handle("GET /admin/", getAdminUI())
	protected := http.Handler(admin)
	if username != "" && password != "" {
		protected = middleware.BasicAuth(username, password)(protected)
	}
	mux.Handle("/api/", protected)
	mux.Handle("/admin/", protected)
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /api/v1/health", app.handleHealthCheck)
	mux.HandleFunc("GET /{shortCode}", app.handleRedirect)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		mux.ServeHTTP(w, r)
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
