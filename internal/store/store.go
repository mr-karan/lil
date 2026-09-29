package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/mr-karan/lil/internal/metrics"
	"github.com/mr-karan/lil/internal/redirect"
	"github.com/mr-karan/lil/models"
	_ "modernc.org/sqlite"
)

//go:embed pragmas.sql
var pragmas string

var ErrNotExist = errors.New("the URL does not exist")
var ErrConflict = errors.New("short code already exists")

// Store commits each mutation before updating its in-memory redirect cache.
type Store struct {
	db          *sql.DB
	logger      *slog.Logger
	shortURLLen int
	cache       map[string]models.URLData
	mu          sync.RWMutex
	writeMu     sync.Mutex
	workers     sync.WaitGroup
	expiryOnce  sync.Once
	expiryStop  context.CancelFunc
}

type Conf struct {
	DBPath         string
	ShortURLLength int
}

func New(cfg Conf, logger *slog.Logger) (*Store, error) {
	if cfg.ShortURLLength < 1 || cfg.ShortURLLength > 128 {
		return nil, fmt.Errorf("short URL length must be between 1 and 128")
	}
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	// One connection keeps connection-scoped PRAGMAs consistent and serializes
	// SQLite writes. Reads are indexed and fetch all device URLs in one query.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(pragmas); err != nil {
		db.Close()
		return nil, err
	}
	if err := runMigrations(db, "migrations", logger); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, logger: logger, shortURLLen: cfg.ShortURLLength, cache: make(map[string]models.URLData)}
	if err := s.loadCache(); err != nil {
		db.Close()
		return nil, err
	}
	metrics.URLsStoredGauge.Set(float64(len(s.cache)))
	return s, nil
}

func (s *Store) Close() error {
	if s.expiryStop != nil {
		s.expiryStop()
	}
	s.workers.Wait()
	return s.db.Close()
}
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM urls`).Scan(&count)
	return count, err
}

func (s *Store) loadCache() error {
	rows, err := s.db.Query(selectURLs)
	if err != nil {
		return err
	}
	urls, err := scanURLs(rows)
	if err != nil {
		return err
	}
	for _, data := range urls {
		s.cache[data.ShortCode] = data
	}
	return nil
}

func (s *Store) CreateShortURL(ctx context.Context, actor Actor, original, title, slug string, expiry time.Duration, devices map[string]string) (string, error) {
	if err := redirect.ValidateDestinations(original, devices); err != nil {
		return "", err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for attempt := 0; attempt < 10; attempt++ {
		code := slug
		if code == "" {
			var err error
			code, err = generateRandomString(s.shortURLLen)
			if err != nil {
				return "", err
			}
		}
		data, err := s.create(ctx, actor, code, original, title, expiry, devices)
		if errors.Is(err, ErrConflict) && slug == "" {
			continue
		}
		if err == nil {
			s.mu.Lock()
			s.cache[code] = data
			metrics.URLsStoredGauge.Set(float64(len(s.cache)))
			s.mu.Unlock()
		}
		return code, err
	}
	return "", fmt.Errorf("could not allocate an unused short code")
}

func (s *Store) create(ctx context.Context, actor Actor, code, original, title string, expiry time.Duration, devices map[string]string) (models.URLData, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.URLData{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var expires *time.Time
	if expiry > 0 {
		end := now.Add(expiry)
		expires = &end
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO urls (short_code, url, title, created_at, expires_at, created_by) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(short_code) DO NOTHING`, code, original, title, now, expires, actor.UserID)
	if err != nil {
		return models.URLData{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return models.URLData{}, err
	}
	if n == 0 {
		return models.URLData{}, ErrConflict
	}
	if err := writeDevices(ctx, tx, code, devices); err != nil {
		return models.URLData{}, err
	}
	data, err := readURL(ctx, tx, code)
	if err != nil {
		return models.URLData{}, err
	}
	if err := writeAudit(ctx, tx, actor, ActionURLCreate, code, nil, snapshotURL(data)); err != nil {
		return models.URLData{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.URLData{}, err
	}
	return data, nil
}

func writeDevices(ctx context.Context, tx *sql.Tx, code string, devices map[string]string) error {
	for platform, target := range devices {
		if target == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO device_urls (short_code, platform, url, created_at) VALUES (?, ?, ?, ?)`, code, platform, target, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

const selectURLs = `SELECT u.short_code, u.url, u.title, u.created_at, u.expires_at, u.updated_at,
 cb.id, cb.email, cb.name, ub.id, ub.email, ub.name,
 d.platform, d.url, d.created_at FROM urls u
 LEFT JOIN users cb ON cb.id = u.created_by
 LEFT JOIN users ub ON ub.id = u.updated_by
 LEFT JOIN device_urls d ON d.short_code = u.short_code `

func scanURLs(rows *sql.Rows) ([]models.URLData, error) {
	defer rows.Close()
	urls := make([]models.URLData, 0)
	indices := make(map[string]int)
	for rows.Next() {
		var data models.URLData
		var expires, updated, created sql.NullTime
		var creatorID, updaterID sql.NullInt64
		var creatorEmail, creatorName, updaterEmail, updaterName, platform, target sql.NullString
		if err := rows.Scan(&data.ShortCode, &data.URL, &data.Title, &data.CreatedAt, &expires, &updated,
			&creatorID, &creatorEmail, &creatorName, &updaterID, &updaterEmail, &updaterName,
			&platform, &target, &created); err != nil {
			return nil, err
		}
		index, exists := indices[data.ShortCode]
		if !exists {
			if expires.Valid {
				data.ExpiresAt = &expires.Time
			}
			if updated.Valid {
				data.UpdatedAt = &updated.Time
			}
			if creatorID.Valid {
				data.CreatedBy = &models.UserRef{ID: creatorID.Int64, Email: creatorEmail.String, Name: creatorName.String}
			}
			if updaterID.Valid {
				data.UpdatedBy = &models.UserRef{ID: updaterID.Int64, Email: updaterEmail.String, Name: updaterName.String}
			}
			data.DeviceURLs = make(map[string]models.DeviceURLData)
			index = len(urls)
			indices[data.ShortCode] = index
			urls = append(urls, data)
		}
		if platform.Valid {
			urls[index].DeviceURLs[platform.String] = models.DeviceURLData{URL: target.String, Platform: platform.String, CreatedAt: created.Time}
		}
	}
	return urls, rows.Err()
}

func readURL(ctx context.Context, tx *sql.Tx, code string) (models.URLData, error) {
	rows, err := tx.QueryContext(ctx, selectURLs+`WHERE u.short_code = ?`, code)
	if err != nil {
		return models.URLData{}, err
	}
	urls, err := scanURLs(rows)
	if err != nil {
		return models.URLData{}, err
	}
	if len(urls) == 0 {
		return models.URLData{}, ErrNotExist
	}
	return urls[0], nil
}

func (s *Store) GetRedirectData(ctx context.Context, code string) (models.URLData, error) {
	s.mu.RLock()
	data, exists := s.cache[code]
	s.mu.RUnlock()
	if !exists {
		return models.URLData{}, ErrNotExist
	}
	if data.ExpiresAt != nil && !time.Now().Before(*data.ExpiresAt) {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		if _, err := s.db.ExecContext(ctx, `DELETE FROM urls WHERE short_code = ?`, code); err != nil {
			return models.URLData{}, err
		}
		s.mu.Lock()
		delete(s.cache, code)
		metrics.URLsStoredGauge.Set(float64(len(s.cache)))
		s.mu.Unlock()
		return models.URLData{}, ErrNotExist
	}
	return data, nil
}

func (s *Store) DeleteURL(ctx context.Context, actor Actor, code string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, err := readURL(ctx, tx, code)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM urls WHERE short_code = ?`, code); err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actor, ActionURLDelete, code, snapshotURL(before), nil); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, code)
	metrics.URLsStoredGauge.Set(float64(len(s.cache)))
	s.mu.Unlock()
	return nil
}

func (s *Store) UpdateURL(ctx context.Context, actor Actor, code, original, title string, devices map[string]string) error {
	if err := redirect.ValidateDestinations(original, devices); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	before, err := readURL(ctx, tx, code)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE urls SET url = ?, title = ?, updated_by = ?, updated_at = ? WHERE short_code = ?`, original, title, actor.UserID, time.Now().UTC(), code); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM device_urls WHERE short_code = ?`, code); err != nil {
		return err
	}
	if err := writeDevices(ctx, tx, code, devices); err != nil {
		return err
	}
	after, err := readURL(ctx, tx, code)
	if err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, actor, ActionURLUpdate, code, snapshotURL(before), snapshotURL(after)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.mu.Lock()
	s.cache[code] = after
	s.mu.Unlock()
	return nil
}

func (s *Store) GetURLs(ctx context.Context, page, perPage int64) ([]models.URLData, int64, error) {
	if page < 1 || page > 1000000 || perPage < 1 || perPage > 1000 {
		return nil, 0, fmt.Errorf("invalid pagination")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM urls`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, selectURLs+`WHERE u.short_code IN (SELECT short_code FROM urls ORDER BY created_at DESC, short_code LIMIT ? OFFSET ?) ORDER BY u.created_at DESC, u.short_code`, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	urls, err := scanURLs(rows)
	return urls, total, err
}

func generateRandomString(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}
