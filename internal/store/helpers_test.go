package store

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(Conf{DBPath: filepath.Join(t.TempDir(), "urls.db"), ShortURLLength: 6}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newUser(t *testing.T, s *Store, email string) User {
	t.Helper()
	u, err := s.UpsertOIDCUser(t.Context(), "https://idp.example", email, email, "")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func newActor(t *testing.T, s *Store) Actor {
	t.Helper()
	return Actor{UserID: newUser(t, s, "alice@example.com").ID}
}
