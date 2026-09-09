package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-karan/lil/internal/store"
)

func TestHTTPRedirectsAndManagement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := store.New(store.Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := &App{store: s, logger: logger}
	router := app.routes("test-user", "test-password")
	ios := "https://apps.apple.com/app/id123456789?action=write-review"
	android := "https://play.google.com/store/apps/details?id=com.example.app"
	if _, err := s.CreateShortURL(context.Background(), "https://example.com/choose", "", "rate-us", 0, map[string]string{"ios": ios, "android": android}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		method, path, ua, want string
		status                 int
	}{
		{"GET", "/rate-us", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", ios, 302},
		{"GET", "/rate-us", "Mozilla/5.0 (Linux; Android 14)", android, 302},
		{"GET", "/rate-us?platform=ios", "", ios, 302},
		{"GET", "/rate-us?platform=web", "", "https://example.com/choose", 302},
		{"HEAD", "/rate-us?platform=android", "", android, 302},
		{"GET", "/rate-us?platform=bad", "", "", 400},
		{"GET", "/missing", "", "", 404},
		{"GET", "/admin", "", "/admin/", 307},
		{"GET", "/admin/", "", "", 401},
		{"GET", "/api/v1/urls", "", "", 401},
		{"GET", "/api/v1/health", "", "", 200},
	} {
		t.Run(tt.method+tt.path+tt.ua, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			r.Header.Set("User-Agent", tt.ua)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tt.status || w.Header().Get("Location") != tt.want {
				t.Fatalf("%d %s", w.Code, w.Header().Get("Location"))
			}
			if tt.status == 302 && (!strings.Contains(w.Header().Get("Cache-Control"), "no-store") || !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "User-Agent")) {
				t.Fatal(w.Header())
			}
			if tt.status == 302 && w.Header().Get("X-Lil-Platform") == "" {
				t.Fatal("missing detected platform header")
			}
		})
	}
	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"url":"javascript:alert(1)"}`, 400},
		{`{"url":"https://example.com","slug":"admin"}`, 400},
		{`{"url":"https://example.com","device_urls":{"windows":"https://example.com"}}`, 400},
		{`{"url":"https://example.com","slug":"rate-us"}`, 409},
		{`{"url":"https://example.com","slug":"new-link"}`, 200},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(tt.body))
		r.SetBasicAuth("test-user", "test-password")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.body, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	r.SetBasicAuth("test-user", "test-password")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-JSON content type: %d", w.Code)
	}
}
