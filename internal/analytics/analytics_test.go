package analytics

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestNewManagerSkipsDisabledProvider(t *testing.T) {
	manager, err := NewManager(Config{
		Enabled:    true,
		NumWorkers: 1,
		Providers: map[string]map[string]interface{}{
			"removed-provider": {"enabled": false},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.dispatchers) != 0 {
		t.Fatalf("initialized %d disabled providers", len(manager.dispatchers))
	}
}

func TestNewManagerRejectsInvalidProviderEnabledValue(t *testing.T) {
	_, err := NewManager(Config{
		Enabled:    true,
		NumWorkers: 1,
		Providers: map[string]map[string]interface{}{
			"accesslog": {"enabled": "false"},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "enabled must be a boolean") {
		t.Fatalf("unexpected error: %v", err)
	}
}
