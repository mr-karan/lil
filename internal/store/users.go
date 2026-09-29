package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrUserDisabled = errors.New("user is disabled")
var ErrUserNotExist = errors.New("the user does not exist")

type UserID int64

type User struct {
	ID           UserID     `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	DisabledAt   *time.Time `json:"disabled_at"`
	Issuer       string     `json:"-"`
	SessionEpoch int64      `json:"-"`
}

// Actor is who performed a mutation. TokenID is nil for browser sessions.
type Actor struct {
	UserID  UserID
	TokenID *TokenID
}

const userColumns = `id, email, name, created_at, last_login_at, disabled_at, oidc_issuer, session_epoch`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(row rowScanner) (User, error) {
	var u User
	var lastLogin, disabled sql.NullTime
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &lastLogin, &disabled, &u.Issuer, &u.SessionEpoch); err != nil {
		return User{}, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	if disabled.Valid {
		u.DisabledAt = &disabled.Time
	}
	return u, nil
}

func (s *Store) UpsertOIDCUser(ctx context.Context, issuer, subject, email, name string) (User, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC()
	u, err := scanUser(s.db.QueryRowContext(ctx, `INSERT INTO users (oidc_issuer, oidc_subject, email, name, created_at, last_login_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(oidc_issuer, oidc_subject) DO UPDATE SET email = excluded.email, name = excluded.name,
		last_login_at = CASE WHEN disabled_at IS NULL THEN excluded.last_login_at ELSE last_login_at END
		RETURNING `+userColumns, issuer, subject, email, name, now, now))
	if err != nil {
		return User{}, err
	}
	if u.DisabledAt != nil {
		return u, ErrUserDisabled
	}
	return u, nil
}

func (s *Store) EnsureDevUser(ctx context.Context, email string) (User, error) {
	return s.UpsertOIDCUser(ctx, "dev", email, email, "")
}

func (s *Store) GetUser(ctx context.Context, id UserID) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotExist
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) SetUserDisabled(ctx context.Context, actor Actor, id UserID, disabled bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query, action := `UPDATE users SET disabled_at = NULL WHERE id = ? RETURNING email`, ActionUserEnable
	args := []any{id}
	if disabled {
		now := time.Now().UTC()
		query, action = `UPDATE users SET disabled_at = COALESCE(disabled_at, ?), session_epoch = session_epoch + 1 WHERE id = ? RETURNING email`, ActionUserDisable
		args = []any{now, id}
	}
	var email string
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotExist
		}
		return err
	}
	if disabled {
		if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, args[0], id); err != nil {
			return err
		}
	}
	if err := writeAudit(ctx, tx, actor, action, targetOf(int64(id)), nil, marshalSnapshot(struct {
		Email string `json:"email"`
	}{email})); err != nil {
		return err
	}
	return tx.Commit()
}
