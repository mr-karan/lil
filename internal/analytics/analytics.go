package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Event represents an analytics event
type Event struct {
	Name       string
	Domain     string
	URL        string
	Referrer   string
	UserAgent  string
	RemoteAddr string
	Timestamp  string
	ShortCode  string
	TargetURL  string
}

// Dispatcher interface that all providers must implement
type Dispatcher interface {
	Send(context.Context, Event) error
	Name() string
	Close() error
}

// Manager handles multiple dispatchers and workers
type Manager struct {
	dispatchers []Dispatcher
	eventChan   chan Event
	logger      *slog.Logger
	numWorkers  int
}

// Config represents analytics configuration
type Config struct {
	Enabled    bool
	NumWorkers int
	Providers  map[string]map[string]interface{}
}

// NewManager creates a new analytics manager
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	m := &Manager{
		eventChan:   make(chan Event, 1000), // buffered channel
		logger:      logger,
		numWorkers:  cfg.NumWorkers,
		dispatchers: make([]Dispatcher, 0),
	}

	// Initialize configured providers
	for providerName, providerConfig := range cfg.Providers {
		dispatcher, err := initializeProvider(providerName, providerConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize provider %s: %w", providerName, err)
		}
		m.dispatchers = append(m.dispatchers, dispatcher)
	}

	return m, nil
}

func initializeProvider(name string, config map[string]interface{}, logger *slog.Logger) (Dispatcher, error) {
	switch name {
	case "plausible":
		cfg := PlausibleConfig{
			Endpoint: config["endpoint"].(string),
			Timeout:  time.Duration(config["timeout"].(int64)) * time.Second,
		}
		return NewPlausibleDispatcher(cfg, logger)
	case "matomo":
		cfg := MatomoConfig{
			TrackingURL: config["tracking_url"].(string),
			SiteID:      int(config["site_id"].(int64)),
			AuthToken:   config["auth_token"].(string),
			Timeout:     time.Duration(config["timeout"].(int64)) * time.Second,
		}
		return NewMatomoDispatcher(cfg, logger)
	case "accesslog":
		return NewAccessLogDispatcher(config, logger)
	case "webhook":
		headers := make(map[string]string)
		if h, ok := config["headers"].(map[string]interface{}); ok {
			for k, v := range h {
				if strVal, ok := v.(string); ok {
					headers[k] = strVal
				}
			}
		}
		cfg := WebhookConfig{
			Endpoint: config["endpoint"].(string),
			Timeout:  time.Duration(config["timeout"].(int64)) * time.Second,
			Headers:  headers,
		}
		return NewWebhookDispatcher(cfg, logger)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}

// Start begins the worker routines
func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.numWorkers; i++ {
		go m.worker(ctx, i)
	}
}

// Track sends an event to the analytics channel
func (m *Manager) Track(evt Event) {
	select {
	case m.eventChan <- evt:
	default:
		m.logger.Warn("analytics channel full, dropping event")
	}
}

// Close cleans up resources
func (m *Manager) Close() error {
	for _, d := range m.dispatchers {
		if err := d.Close(); err != nil {
			m.logger.Error("failed to close dispatcher",
				"provider", d.Name(),
				"error", err)
		}
	}
	return nil
}

// worker processes events from the channel
func (m *Manager) worker(ctx context.Context, id int) {
	m.logger.Info("starting analytics worker", "worker_id", id)

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-m.eventChan:
			for _, d := range m.dispatchers {
				if err := d.Send(ctx, evt); err != nil {
					m.logger.Error("failed to send event",
						"provider", d.Name(),
						"error", err)
				}
			}
		}
	}
}
