package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/mr-karan/lil/internal/analytics"
	"github.com/mr-karan/lil/internal/auth"
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
	if err := run(); err != nil {
		slog.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	initConfig()
	app := &App{
		logger: initLogger(ko.Bool("app.enable_debug_logs")),
	}

	// Initialize SQLite store.
	store, err := store.New(store.Conf{
		DBPath:         ko.MustString("db.path"),
		ShortURLLength: ko.MustInt("app.short_url_length"),
	}, app.logger)
	if err != nil {
		return fmt.Errorf("initialize SQLite store: %w", err)
	}
	defer store.Close()

	app.store = store

	// Initialize analytics manager. The [analytics] section is optional.
	analyticsConfig := analytics.Config{Enabled: ko.Bool("analytics.enabled")}
	if analyticsConfig.Enabled {
		analyticsConfig.NumWorkers = ko.MustInt("analytics.num_workers")
		analyticsConfig.Providers = make(map[string]map[string]interface{})
		if providersRaw := ko.Get("analytics.providers"); providersRaw != nil {
			providerValues, ok := providersRaw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("analytics.providers must be a table")
			}
			for provider, config := range providerValues {
				if configMap, ok := config.(map[string]interface{}); ok {
					analyticsConfig.Providers[provider] = configMap
				}
			}
		}
	}

	analyticsManager, err := analytics.NewManager(analyticsConfig, app.logger)
	if err != nil {
		return fmt.Errorf("initialize analytics: %w", err)
	}
	app.analytics = analyticsManager

	// Start analytics workers for dispatching events.
	if analyticsManager != nil {
		analyticsCtx, cancelAnalytics := context.WithCancel(context.Background())
		analyticsManager.Start(analyticsCtx)
		defer func() {
			cancelAnalytics()
			analyticsManager.Close()
		}()
	}

	authn, err := auth.New(context.Background(), auth.Config{
		Mode:            auth.Mode(ko.String("auth.mode")),
		DevEmail:        ko.String("auth.dev_email"),
		SessionLifetime: ko.Duration("auth.session_lifetime"),
		IssuerURL:       ko.String("auth.oidc.issuer_url"),
		ClientID:        ko.String("auth.oidc.client_id"),
		ClientSecret:    ko.String("auth.oidc.client_secret"),
		RedirectURL:     ko.String("auth.oidc.redirect_url"),
		AllowedDomains:  ko.Strings("auth.oidc.allowed_domains"),
		AllowedEmails:   ko.Strings("auth.oidc.allowed_emails"),
	}, app.store, app.logger)
	if err != nil {
		return fmt.Errorf("initialize auth: %w", err)
	}
	server := &http.Server{
		Addr:              ko.MustString("server.address"),
		Handler:           app.routes(authn),
		ReadTimeout:       ko.MustDuration("server.read_timeout"),
		WriteTimeout:      ko.MustDuration("server.write_timeout"),
		IdleTimeout:       ko.MustDuration("server.idle_timeout"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start URL expiry worker
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.store.StartExpiryWorker(ctx)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			app.logger.Error("HTTP shutdown failed", "error", err)
			server.Close()
		}
	}()

	app.logger.Info("starting server", "address", server.Addr, "build", buildString)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
		<-shutdownDone
		return fmt.Errorf("serve HTTP: %w", err)
	}
	stop()
	<-shutdownDone
	return nil
}
