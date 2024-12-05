package analytics

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type MatomoConfig struct {
	TrackingURL string
	SiteID      int
	AuthToken   string
	Timeout     time.Duration
}

type MatomoDispatcher struct {
	config MatomoConfig
	client *http.Client
	logger *slog.Logger
}

func NewMatomoDispatcher(config MatomoConfig, logger *slog.Logger) (*MatomoDispatcher, error) {
	if config.TrackingURL == "" {
		return nil, fmt.Errorf("matomo tracking URL is required")
	}
	if config.SiteID == 0 {
		return nil, fmt.Errorf("matomo site ID is required")
	}
	if config.Timeout == 0 {
		return nil, fmt.Errorf("matomo timeout is required")
	}

	return &MatomoDispatcher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}, nil
}

func (m *MatomoDispatcher) Name() string {
	return "matomo"
}

func (m *MatomoDispatcher) Send(ctx context.Context, evt Event) error {
	params := url.Values{}

	// Required parameters
	params.Set("idsite", strconv.Itoa(m.config.SiteID))
	params.Set("rec", "1")
	params.Set("apiv", "1")

	// TODO: Make protocol configurable instead of hardcoding to https
	shortURL := fmt.Sprintf("%s/%s", evt.Domain, evt.ShortCode)
	params.Set("url", "https://"+shortURL)
	params.Set("action_name", fmt.Sprintf("Redirect to: %s", evt.TargetURL))

	// Event tracking
	params.Set("e_c", "Shortlink")   // Category
	params.Set("e_a", "Redirect")    // Action
	params.Set("e_n", evt.ShortCode) // Name

	// User info
	params.Set("urlref", evt.Referrer) // Referrer URL
	params.Set("ua", evt.UserAgent)    // User Agent

	// Generate random value to avoid caching
	params.Set("rand", strconv.FormatInt(time.Now().UnixNano(), 10))

	// Set the client IP if auth token is available (required for IP tracking)
	if m.config.AuthToken != "" {
		if evt.UserIP != "" {
			params.Set("cip", evt.UserIP)
		}
		params.Set("token_auth", m.config.AuthToken)
	}

	// Construct the final URL
	trackingURL := fmt.Sprintf("%s?%s", m.config.TrackingURL, params.Encode())

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", trackingURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add parameter to avoid receiving GIF image
	params.Set("send_image", "0")

	// Log all request parameters
	m.logger.Info("sending matomo request",
		"url", trackingURL,
		"params", params,
		"user_agent", evt.UserAgent,
		"user_ip", evt.UserIP)

	// Send request
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		// Read response body for error details
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("matomo request failed with status: %d, failed to read response body: %v", resp.StatusCode, err)
		}
		return fmt.Errorf("matomo request failed with status: %d, response: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (m *MatomoDispatcher) Close() error {
	return nil
}
