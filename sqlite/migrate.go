package sqlite

import (
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MigrateUp runs the up migrations in dir of fsys.
func MigrateUp(db *sql.DB, fsys fs.FS, dir string) error {
	return runMigrate(db, fsys, dir, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil {
			if errors.Is(err, migrate.ErrNoChange) {
				slog.Info("migrations already up to date")
				return nil
			}
			return err
		}
		return nil
	})
}

// MigrateDown runs the down migrations in dir of fsys.
func MigrateDown(db *sql.DB, fsys fs.FS, dir string) error {
	return runMigrate(db, fsys, dir, func(m *migrate.Migrate) error {
		return m.Down()
	})
}

func runMigrate(db *sql.DB, fsys fs.FS, dir string, f func(*migrate.Migrate) error) error {
	sourceInstance, err := iofs.New(fsys, dir)
	if err != nil {
		return err
	}

	defer sourceInstance.Close()

	dbInstance, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceInstance, "sqlite", dbInstance)
	if err != nil {
		return err
	}

	return f(m)
}
