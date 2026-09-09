package analytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMatomoErrorsDoNotExposeRequestSecrets(t *testing.T) {
	const token = "sensitive-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token_auth") != token {
			t.Error("Matomo authentication token was not sent")
		}
		if r.URL.Query().Get("send_image") != "0" {
			t.Error("send_image was not sent")
		}
		http.Error(w, "sensitive upstream response", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	dispatcher, err := NewMatomoDispatcher(MatomoConfig{
		TrackingURL: server.URL,
		SiteID:      17,
		AuthToken:   token,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dispatcher.Send(context.Background(), Event{Domain: "links.example.com", ShortCode: "rate-us"})
	if err == nil {
		t.Fatal("expected Matomo request to fail")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "sensitive upstream response") {
		t.Fatalf("error exposed sensitive data: %q", err)
	}
}
