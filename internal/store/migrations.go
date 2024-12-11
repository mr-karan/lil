package store

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/mr-karan/lil/migrations"
)

func runMigrations(db *sql.DB, migrationsPath string, logger *slog.Logger) error {
	logger.Info("starting database migrations", "path", migrationsPath)
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("could not create sqlite driver: %w", err)
	}

	d, err := iofs.New(migrations.MigrationsFS, ".")
	if err != nil {
		return fmt.Errorf("could not create source driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		d,
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
