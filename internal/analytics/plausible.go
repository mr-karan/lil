package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type PlausibleConfig struct {
	Endpoint string
	Timeout  time.Duration
}

type PlausibleDispatcher struct {
	config PlausibleConfig
	client *http.Client
	logger *slog.Logger
}

type plausibleEvent struct {
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	URL      string `json:"url"`
	Referrer string `json:"referrer,omitempty"`
}

func NewPlausibleDispatcher(config PlausibleConfig, logger *slog.Logger) (*PlausibleDispatcher, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("plausible endpoint is required")
	}
	if config.Timeout == 0 {
		return nil, fmt.Errorf("plausible timeout is required")
	}

	return &PlausibleDispatcher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}, nil
}

func (p *PlausibleDispatcher) Name() string {
	return "plausible"
}

func (p *PlausibleDispatcher) Send(ctx context.Context, evt Event) error {
	plEvent := plausibleEvent{
		Name:     "pageview",
		Domain:   evt.Domain,
		URL:      evt.URL,
		Referrer: evt.Referrer,
	}

	jsonData, err := json.Marshal(plEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", evt.UserAgent)
	req.Header.Set("X-Forwarded-For", evt.UserIP)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("plausible request failed with status: %d", resp.StatusCode)
	}

	return nil
}

// noop
func (p *PlausibleDispatcher) Close() error {
	return nil
}
