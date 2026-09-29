package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/alexedwards/scs/v2"
)

var _ scs.Store = (*SessionStore)(nil)

// SessionStore implements scs.Store on the shared database handle.
type SessionStore struct{ db *sql.DB }

func (s *Store) SessionStore() *SessionStore { return &SessionStore{db: s.db} }

func (s *SessionStore) Find(token string) ([]byte, bool, error) {
	var data []byte
	err := s.db.QueryRowContext(context.Background(), `SELECT data FROM sessions WHERE token = ? AND julianday('now') < expiry`, token).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *SessionStore) Commit(token string, b []byte, expiry time.Time) error {
	_, err := s.db.ExecContext(context.Background(), `INSERT INTO sessions (token, data, expiry) VALUES (?, ?, julianday(?))
		ON CONFLICT(token) DO UPDATE SET data = excluded.data, expiry = excluded.expiry`, token, b, expiry.UTC().Format("2006-01-02T15:04:05.000"))
	return err
}

func (s *SessionStore) Delete(token string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM sessions WHERE token = ?`, token)
	return err
}
