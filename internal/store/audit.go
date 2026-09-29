package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/mr-karan/lil/models"
)

type AuditAction string

const (
	ActionURLCreate   AuditAction = "url.create"
	ActionURLUpdate   AuditAction = "url.update"
	ActionURLDelete   AuditAction = "url.delete"
	ActionTokenCreate AuditAction = "token.create"
	ActionTokenRevoke AuditAction = "token.revoke"
	ActionUserDisable AuditAction = "user.disable"
	ActionUserEnable  AuditAction = "user.enable"
)

type AuditEntry struct {
	ID        int64           `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Actor     models.UserRef  `json:"actor"`
	TokenID   *TokenID        `json:"token_id"`
	Action    AuditAction     `json:"action"`
	Target    string          `json:"target"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
}

type urlSnapshot struct {
	URL        string            `json:"url"`
	Title      string            `json:"title"`
	ExpiresAt  *time.Time        `json:"expires_at"`
	DeviceURLs map[string]string `json:"device_urls"`
}

func snapshotURL(data models.URLData) json.RawMessage {
	devices := make(map[string]string, len(data.DeviceURLs))
	for platform, device := range data.DeviceURLs {
		devices[platform] = device.URL
	}
	return marshalSnapshot(urlSnapshot{URL: data.URL, Title: data.Title, ExpiresAt: data.ExpiresAt, DeviceURLs: devices})
}

func marshalSnapshot(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal audit snapshot: %v", err))
	}
	return b
}

func writeAudit(ctx context.Context, tx *sql.Tx, actor Actor, action AuditAction, target string, before, after json.RawMessage) error {
	var beforeText, afterText *string
	if before != nil {
		text := string(before)
		beforeText = &text
	}
	if after != nil {
		text := string(after)
		afterText = &text
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_log (created_at, user_id, token_id, action, target, before, after) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC(), actor.UserID, actor.TokenID, action, target, beforeText, afterText)
	return err
}

func (s *Store) ListAudit(ctx context.Context, shortCode string, page, perPage int64) ([]AuditEntry, int64, error) {
	if page < 1 || page > 1000000 || perPage < 1 || perPage > 1000 {
		return nil, 0, fmt.Errorf("invalid pagination")
	}
	filter, args := "", []any{}
	if shortCode != "" {
		filter, args = ` WHERE a.action LIKE 'url.%' AND a.target = ?`, []any{shortCode}
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log a`+filter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.created_at, u.id, u.email, u.name, a.token_id, a.action, a.target, a.before, a.after
		FROM audit_log a JOIN users u ON u.id = a.user_id`+filter+` ORDER BY a.id DESC LIMIT ? OFFSET ?`,
		append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		var tokenID sql.NullInt64
		var before, after sql.NullString
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Actor.ID, &e.Actor.Email, &e.Actor.Name, &tokenID, &e.Action, &e.Target, &before, &after); err != nil {
			return nil, 0, err
		}
		if tokenID.Valid {
			id := TokenID(tokenID.Int64)
			e.TokenID = &id
		}
		if before.Valid {
			e.Before = json.RawMessage(before.String)
		}
		if after.Valid {
			e.After = json.RawMessage(after.String)
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

func targetOf(id int64) string { return strconv.FormatInt(id, 10) }
