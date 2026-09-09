package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type AccessLogDispatcher struct {
	logger      *slog.Logger
	fileHandler slog.Handler
	fileWriter  *os.File
}

func NewAccessLogDispatcher(cfg map[string]interface{}, logger *slog.Logger) (*AccessLogDispatcher, error) {
	var fileWriter *os.File
	var fileHandler slog.Handler

	if filePath, ok := cfg["file_path"].(string); ok && filePath != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
			return nil, fmt.Errorf("create access log directory: %w", err)
		}

		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("open access log: %w", err)
		}
		fileWriter = f
		fileHandler = slog.NewJSONHandler(f, nil)
	}

	return &AccessLogDispatcher{
		logger:      logger,
		fileHandler: fileHandler,
		fileWriter:  fileWriter,
	}, nil
}

func (a *AccessLogDispatcher) Name() string {
	return "accesslog"
}

func (a *AccessLogDispatcher) Send(ctx context.Context, evt Event) error {
	attrs := []any{
		"provider", a.Name(),
		"event_name", evt.Name,
		"short_code", evt.ShortCode,
		"domain", evt.Domain,
		"client_ip", evt.UserIP,
		"remote_addr", evt.RemoteAddr,
		"referrer_host", referrerHost(evt.Referrer),
		"user_agent", evt.UserAgent,
		"event_time", evt.Timestamp,
	}
	a.logger.InfoContext(ctx, "redirect accessed", attrs...)
	if a.fileHandler != nil {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "redirect accessed", 0)
		record.Add(attrs...)
		if err := a.fileHandler.Handle(ctx, record); err != nil {
			return fmt.Errorf("write access log: %w", err)
		}
	}
	return nil
}

func referrerHost(raw string) string {
	referrer, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return referrer.Hostname()
}

func (a *AccessLogDispatcher) Close() error {
	if a.fileWriter != nil {
		return a.fileWriter.Close()
	}
	return nil
}
