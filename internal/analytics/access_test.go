package analytics

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccessLogDispatcherWritesStructuredLogs(t *testing.T) {
	var output bytes.Buffer
	filePath := filepath.Join(t.TempDir(), "access.jsonl")
	dispatcher, err := NewAccessLogDispatcher(
		map[string]interface{}{"file_path": filePath},
		slog.New(slog.NewJSONHandler(&output, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}

	event := Event{
		Name:       "pageview",
		Domain:     "links.example.com",
		UserIP:     "192.0.2.1",
		RemoteAddr: "127.0.0.1:1234",
		Referrer:   "https://example.com/path?token=secret-referrer",
		UserAgent:  "ExampleBrowser/1",
		Timestamp:  "2026-09-09T10:00:00Z",
		ShortCode:  "review",
		TargetURL:  "https://apps.example.com/app?id=secret",
	}
	if err := dispatcher.Send(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Close(); err != nil {
		t.Fatal(err)
	}

	fileOutput, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, logLine := range []string{output.String(), string(fileOutput)} {
		for _, want := range []string{
			`"msg":"redirect accessed"`,
			`"provider":"accesslog"`,
			`"short_code":"review"`,
			`"client_ip":"192.0.2.1"`,
			`"referrer_host":"example.com"`,
			`"user_agent":"ExampleBrowser/1"`,
		} {
			if !strings.Contains(logLine, want) {
				t.Errorf("log does not contain %s: %s", want, logLine)
			}
		}
		if strings.Contains(logLine, "secret") {
			t.Errorf("log exposed destination query value: %s", logLine)
		}
	}
}
