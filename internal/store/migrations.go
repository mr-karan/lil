package store

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func runMigrations(db *sql.DB, migrationsPath string, logger *slog.Logger) error {
	logger.Info("starting database migrations", "path", migrationsPath)
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("could not create sqlite driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"sqlite",
		driver,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("no new migrations to apply")
			return nil
		}
		return fmt.Errorf("could not run migrations: %w", err)
	}

	logger.Info("database migrations completed successfully")
	return nil
}
