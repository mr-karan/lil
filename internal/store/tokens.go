package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

var ErrUnauthorized = errors.New("invalid or revoked API token")
var ErrTokenNotExist = errors.New("the API token does not exist")

const tokenPrefixLen = 12

type TokenID int64

type APIToken struct {
	ID          TokenID   `json:"id"`
	Name        string    `json:"name"`
	TokenPrefix string    `json:"token_prefix"`
	CreatedAt   time.Time `json:"created_at"`
}

type tokenSnapshot struct {
	Name        string `json:"name"`
	TokenPrefix string `json:"token_prefix"`
}

func hashToken(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}

func (s *Store) CreateAPIToken(ctx context.Context, actor Actor, name string) (string, APIToken, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", APIToken{}, err
	}
	plaintext := "lil_" + base64.RawURLEncoding.EncodeToString(secret)
	token := APIToken{Name: name, TokenPrefix: plaintext[:tokenPrefixLen], CreatedAt: time.Now().UTC()}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", APIToken{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, created_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		actor.UserID, token.Name, hashToken(plaintext), token.TokenPrefix, token.CreatedAt).Scan(&token.ID); err != nil {
		return "", APIToken{}, err
	}
	if err := writeAudit(ctx, tx, actor, ActionTokenCreate, targetOf(int64(token.ID)), nil, marshalSnapshot(tokenSnapshot{token.Name, token.TokenPrefix})); err != nil {
		return "", APIToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return "", APIToken{}, err
	}
	return plaintext, token, nil
}

func (s *Store) ListAPITokens(ctx context.Context, userID UserID) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, token_prefix, created_at FROM api_tokens WHERE user_id = ? AND revoked_at IS NULL ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]APIToken, 0)
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenPrefix, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *Store) RevokeAPIToken(ctx context.Context, actor Actor, id TokenID) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var snapshot tokenSnapshot
	err = tx.QueryRowContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL RETURNING name, token_prefix`,
		time.Now().UTC(), id, actor.UserID).Scan(&snapshot.Name, &snapshot.TokenPrefix)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTokenNotExist
	}
	if err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actor, ActionTokenRevoke, targetOf(int64(id)), marshalSnapshot(snapshot), nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AuthenticateAPIToken(ctx context.Context, plaintext string) (Actor, User, error) {
	var tokenID TokenID
	var lastLogin, disabled sql.NullTime
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT t.id, u.id, u.email, u.name, u.created_at, u.last_login_at, u.disabled_at, u.oidc_issuer, u.session_epoch
		FROM api_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ? AND t.revoked_at IS NULL`, hashToken(plaintext)).
		Scan(&tokenID, &u.ID, &u.Email, &u.Name, &u.CreatedAt, &lastLogin, &disabled, &u.Issuer, &u.SessionEpoch)
	if errors.Is(err, sql.ErrNoRows) || disabled.Valid {
		return Actor{}, User{}, ErrUnauthorized
	}
	if err != nil {
		return Actor{}, User{}, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return Actor{UserID: u.ID, TokenID: &tokenID}, u, nil
}
