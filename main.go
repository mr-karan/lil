package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/VictoriaMetrics/metrics"
	"github.com/knadh/koanf/v2"
	"github.com/mr-karan/lil/internal/analytics"
	"github.com/mr-karan/lil/internal/middleware"
	"github.com/mr-karan/lil/internal/store"
)

type App struct {
	store     *store.Store
	logger    *slog.Logger
	analytics *analytics.Manager
}

var (
	ko          = koanf.New(".")
	buildString = "unknown"
)

func main() {
	app := &App{
		logger: initLogger(ko.Bool("app.enable_debug_logs")),
	}

	// Initialize SQLite store.
	store, err := store.New(store.Conf{
		DBPath:              ko.MustString("db.path"),
		MaxOpenConns:        ko.MustInt("db.max_open_conns"),
		MaxIdleConns:        ko.MustInt("db.max_idle_conns"),
		ConnMaxLifetimeMins: ko.MustInt("db.conn_max_lifetime_mins"),
		ShortURLLength:      ko.MustInt("app.short_url_length"),
		BufferSize:          ko.MustInt("db.buffer_size"),
		FlushInterval:       ko.MustDuration("db.flush_interval"),
	}, app.logger)
	if err != nil {
		app.logger.Error("Failed to initialize SQLite store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	app.store = store

	// Initialize analytics manager.
	providers := make(map[string]map[string]interface{})
	if providersRaw := ko.Get("analytics.providers"); providersRaw != nil {
		for provider, config := range providersRaw.(map[string]interface{}) {
			if configMap, ok := config.(map[string]interface{}); ok {
				providers[provider] = configMap
			}
		}
	}

	analyticsConfig := analytics.Config{
		Enabled:    ko.Bool("analytics.enabled"),
		NumWorkers: ko.MustInt("analytics.num_workers"),
		Providers:  providers,
	}

	analyticsManager, err := analytics.NewManager(analyticsConfig, app.logger)
	if err != nil {
		app.logger.Error("Failed to initialize analytics", "error", err)
		os.Exit(1)
	}
	app.analytics = analyticsManager

	// Start analytics workers for dispatching events.
	analyticsManager.Start(context.TODO())

	// Initialize router and start server
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/v1", app.handleIndex)
	mux.HandleFunc("GET /api/v1/health", app.handleHealthCheck)
	mux.HandleFunc("POST /api/v1/shorten", app.handleShortenURL)
	mux.HandleFunc("GET /api/v1/urls", app.handleGetURLs)
	mux.HandleFunc("PUT /api/v1/urls/{shortCode}", app.handleUpdateURL)
	mux.HandleFunc("DELETE /api/v1/urls/{shortCode}", app.handleDeleteURL)
	mux.HandleFunc("GET /api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.WritePrometheus(w, true)
	})

	// Admin UI routes with basic auth
	adminHandler := getAdminUI()
	if username, password := ko.String("admin.username"), ko.String("admin.password"); username != "" && password != "" {
		adminHandler = middleware.BasicAuth(username, password)(adminHandler)
	}
	mux.Handle("GET /admin/", adminHandler)
	mux.Handle("GET /admin/...", adminHandler)

	// Short URL redirect handler (catch-all)
	mux.HandleFunc("GET /{shortCode}", app.handleRedirect)

	server := &http.Server{
		Addr:         ko.MustString("server.address"),
		Handler:      mux,
		ReadTimeout:  ko.MustDuration("server.read_timeout"),
		WriteTimeout: ko.MustDuration("server.write_timeout"),
		IdleTimeout:  ko.MustDuration("server.idle_timeout"),
	}

	// Start URL expiry worker
	app.store.StartExpiryWorker(context.Background())

	app.logger.Info("starting server", "address", server.Addr, "build", buildString)
	if err := server.ListenAndServe(); err != nil {
		app.logger.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
