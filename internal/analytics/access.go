package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type AccessLogDispatcher struct {
	logger     *slog.Logger
	fileWriter *os.File
}

func NewAccessLogDispatcher(cfg map[string]interface{}, logger *slog.Logger) (*AccessLogDispatcher, error) {
	var fileWriter *os.File

	if filePath, ok := cfg["file_path"].(string); ok && filePath != "" {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		// Open log file in append mode
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		fileWriter = f
	}

	return &AccessLogDispatcher{
		logger:     logger,
		fileWriter: fileWriter,
	}, nil
}

func (a *AccessLogDispatcher) Name() string {
	return "accesslog"
}

func (a *AccessLogDispatcher) formatLogEntry(evt Event) string {
	// Format timestamp in Apache log format
	timestamp := time.Now().Format("02/Jan/2006:15:04:05 -0700")

	// Construct the log entry in Combined Log Format
	// %h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-Agent}i"
	logEntry := fmt.Sprintf("%s (%s) - - [%s] \"GET /%s HTTP/1.1\" 302 - \"%s\" \"%s\"\n",
		evt.RemoteAddr,
		evt.UserIP,
		timestamp,
		evt.ShortCode,
		evt.Referrer,
		evt.UserAgent,
	)

	return logEntry
}

func (a *AccessLogDispatcher) Send(ctx context.Context, evt Event) error {
	logEntry := a.formatLogEntry(evt)

	// Write to stdout
	fmt.Print(logEntry)

	// Write to file if configured
	if a.fileWriter != nil {
		if _, err := a.fileWriter.WriteString(logEntry); err != nil {
			return fmt.Errorf("failed to write to log file: %w", err)
		}
	}

	return nil
}

func (a *AccessLogDispatcher) Close() error {
	if a.fileWriter != nil {
		return a.fileWriter.Close()
	}
	return nil
}
