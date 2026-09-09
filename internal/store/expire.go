package store

import (
	"context"
	"time"

	"github.com/mr-karan/lil/internal/metrics"
)

// StartExpiryWorker starts a background goroutine that periodically checks and removes expired URLs
func (s *Store) StartExpiryWorker(ctx context.Context) {
	s.expiryOnce.Do(func() {
		workerCtx, stop := context.WithCancel(ctx)
		s.expiryStop = stop
		ticker := time.NewTicker(24 * time.Hour)
		s.workers.Go(func() {
			defer ticker.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
					if err := s.removeExpiredURLs(workerCtx); err != nil {
						s.logger.Error("failed to remove expired URLs", "error", err)
					}
				}
			}
		})
		s.logger.Info("started URL expiry worker")
	})
}

// removeExpiredURLs removes all expired URLs from both the database and cache
func (s *Store) removeExpiredURLs(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.db.QueryContext(ctx,
		`DELETE FROM urls
		 WHERE expires_at IS NOT NULL
		 AND julianday(expires_at) <= julianday('now')
		 RETURNING short_code`)
	if err != nil {
		return err
	}
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return err
		}
		codes = append(codes, code)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	for _, code := range codes {
		delete(s.cache, code)
	}
	metrics.URLsStoredGauge.Set(float64(len(s.cache)))
	s.mu.Unlock()
	return nil
}
