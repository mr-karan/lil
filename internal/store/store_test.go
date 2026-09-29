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

	"github.com/mr-karan/lil/migrations"
)

func TestPersistenceAndDeviceUpdates(t *testing.T) {
	cfg := Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	actor := newActor(t, s)
	if _, err := s.CreateShortURL(ctx, actor, "https://example.com", "test", "rate-us", 0, nil); err != nil {
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
	devices := map[string]string{"ios": "https://apps.apple.com/app/id123456789?action=write-review", "web": "https://example.com/web"}
	if err := s.UpdateURL(ctx, actor, "rate-us", "https://example.com/new", "updated", devices); err != nil {
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
	if _, err := s.CreateShortURL(ctx, actor, "https://example.com", "", "rate-us", 0, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.DeleteURL(ctx, actor, "rate-us"); err != nil {
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
	actor := newActor(t, s)
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			_, err := s.CreateShortURL(ctx, actor, "https://example.com", "", "same", 0, nil)
			results <- err
		})
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
	if err := s.DeleteURL(ctx, actor, "same"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateShortURL(ctx, actor, "https://example.com", "", "expired", time.Nanosecond, nil); err != nil {
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

func TestMutationsRecordAttributionAndAudit(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	alice := newActor(t, s)
	bob := Actor{UserID: newUser(t, s, "bob@example.com").ID}

	if _, err := s.CreateShortURL(ctx, alice, "https://example.com/a", "first", "doc", 0, map[string]string{"ios": "https://apps.apple.com/app/id1"}); err != nil {
		t.Fatal(err)
	}
	data, err := s.GetRedirectData(ctx, "doc")
	if err != nil || data.CreatedBy == nil || data.CreatedBy.Email != "alice@example.com" || data.UpdatedBy != nil || data.UpdatedAt != nil {
		t.Fatalf("%+v %v", data, err)
	}
	if err := s.UpdateURL(ctx, bob, "doc", "https://example.com/b", "second", nil); err != nil {
		t.Fatal(err)
	}
	data, err = s.GetRedirectData(ctx, "doc")
	if err != nil || data.UpdatedBy == nil || data.UpdatedBy.Email != "bob@example.com" || data.UpdatedAt == nil || data.CreatedBy.Email != "alice@example.com" {
		t.Fatalf("%+v %v", data, err)
	}
	urls, _, err := s.GetURLs(ctx, 1, 10)
	if err != nil || len(urls) != 1 || urls[0].UpdatedBy == nil || urls[0].UpdatedBy.ID != int64(bob.UserID) {
		t.Fatalf("%+v %v", urls, err)
	}
	if err := s.DeleteURL(ctx, alice, "doc"); err != nil {
		t.Fatal(err)
	}

	entries, total, err := s.ListAudit(ctx, "doc", 1, 10)
	if err != nil || total != 3 || len(entries) != 3 {
		t.Fatalf("%+v %d %v", entries, total, err)
	}
	wantActions := []AuditAction{ActionURLDelete, ActionURLUpdate, ActionURLCreate}
	for i, want := range wantActions {
		if entries[i].Action != want || entries[i].Target != "doc" {
			t.Fatalf("entry %d: %+v", i, entries[i])
		}
	}
	if entries[0].Actor.Email != "alice@example.com" || entries[1].Actor.Email != "bob@example.com" {
		t.Fatalf("actors: %+v", entries)
	}
	const (
		createdJSON = `{"url":"https://example.com/a","title":"first","expires_at":null,"device_urls":{"ios":"https://apps.apple.com/app/id1"}}`
		updatedJSON = `{"url":"https://example.com/b","title":"second","expires_at":null,"device_urls":{}}`
	)
	checks := []struct {
		name      string
		got, want string
	}{
		{"create before", string(entries[2].Before), ""},
		{"create after", string(entries[2].After), createdJSON},
		{"update before", string(entries[1].Before), createdJSON},
		{"update after", string(entries[1].After), updatedJSON},
		{"delete before", string(entries[0].Before), updatedJSON},
		{"delete after", string(entries[0].After), ""},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q want %q", c.name, c.got, c.want)
		}
	}
}

func TestAuditFailureRollsBackMutation(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	alice := newActor(t, s)
	ghost := Actor{UserID: alice.UserID + 100}

	if _, err := s.CreateShortURL(ctx, ghost, "https://example.com", "", "ghost", 0, nil); err == nil {
		t.Fatal("create with unknown actor succeeded")
	}
	if _, err := s.GetRedirectData(ctx, "ghost"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("cache entry after failed create: %v", err)
	}

	if _, err := s.CreateShortURL(ctx, alice, "https://example.com", "", "kept", 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteURL(ctx, ghost, "kept"); err == nil {
		t.Fatal("delete with unknown actor succeeded")
	}
	if err := s.UpdateURL(ctx, ghost, "kept", "https://example.com/x", "", nil); err == nil {
		t.Fatal("update with unknown actor succeeded")
	}
	data, err := s.GetRedirectData(ctx, "kept")
	if err != nil || data.URL != "https://example.com" || data.UpdatedBy != nil {
		t.Fatalf("cache after failed mutations: %+v %v", data, err)
	}
	urls, count, err := s.GetURLs(ctx, 1, 10)
	if err != nil || count != 1 || urls[0].URL != "https://example.com" {
		t.Fatalf("db after failed mutations: %+v %d %v", urls, count, err)
	}
	if _, total, err := s.ListAudit(ctx, "", 1, 10); err != nil || total != 1 {
		t.Fatalf("audit rows=%d: %v", total, err)
	}
}

func TestMigrationsDownThenUp(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateShortURL(t.Context(), newActor(t, s), "https://example.com", "", "x", 0, map[string]string{"web": "https://example.com/w"}); err != nil {
		t.Fatal(err)
	}
	down, err := migrations.MigrationsFS.ReadFile("000002_multi_user.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := migrations.MigrationsFS.ReadFile("000002_multi_user.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(string(down)); err != nil {
		t.Fatal(err)
	}
	var urls, devices, columns int
	if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM urls), (SELECT COUNT(*) FROM device_urls), (SELECT COUNT(*) FROM pragma_table_info('urls') WHERE name = 'created_by')`).Scan(&urls, &devices, &columns); err != nil || urls != 1 || devices != 1 || columns != 0 {
		t.Fatalf("urls=%d devices=%d created_by=%d: %v", urls, devices, columns, err)
	}
	if _, err := s.db.Exec(string(up)); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('urls') WHERE name IN ('created_by', 'updated_by', 'updated_at')`).Scan(&columns); err != nil || columns != 3 {
		t.Fatalf("columns after up=%d: %v", columns, err)
	}
}
