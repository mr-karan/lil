package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPersistenceAndDeviceUpdates(t *testing.T) {
	cfg := Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.CreateShortURL(ctx, "https://example.com", "test", "rate-us", 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	devices := map[string]string{"ios": "https://apps.apple.com/app/id1449453802?action=write-review", "web": "https://example.com/web"}
	if err := s.UpdateURL(ctx, "rate-us", "https://example.com/new", "updated", devices); err != nil {
		t.Fatal(err)
	}
	data, err := s.GetRedirectData(ctx, "rate-us")
	if err != nil || data.DeviceURLs["ios"].URL != devices["ios"] || data.DeviceURLs["web"].URL != devices["web"] {
		t.Fatalf("%+v %v", data, err)
	}
	urls, count, err := s.GetURLs(ctx, 1, 20)
	if err != nil || count != 1 || len(urls) != 1 || len(urls[0].DeviceURLs) != 2 {
		t.Fatalf("%+v %d %v", urls, count, err)
	}
	if _, err := s.CreateShortURL(ctx, "https://example.com", "", "rate-us", 0, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.DeleteURL(ctx, "rate-us"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRedirectData(ctx, "rate-us"); !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}
	var remaining int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM device_urls`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("orphan device rows=%d: %v", remaining, err)
	}
}

func TestConcurrentSlugAndExpiry(t *testing.T) {
	s, err := New(Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() { _, err := s.CreateShortURL(ctx, "https://example.com", "", "same", 0, nil); results <- err })
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d successful duplicate creates", winners)
	}
	if err := s.DeleteURL(ctx, "same"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateShortURL(ctx, "https://example.com", "", "expired", time.Nanosecond, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := s.GetRedirectData(ctx, "expired"); !errors.Is(err, ErrNotExist) {
		t.Fatal(err)
	}
}

func TestCloseStopsExpiryWorker(t *testing.T) {
	s, err := New(Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	s.StartExpiryWorker(context.Background())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
