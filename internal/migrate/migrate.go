package migrate

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up applies all pending migrations.
func Up(dsn string) error {
	m, err := newMigrate(dsn)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// Down rolls back n migrations.
func Down(dsn string, steps int) error {
	m, err := newMigrate(dsn)
	if err != nil {
		return err
	}
	if err := m.Steps(-steps); err != nil {
		return err
	}
	return nil
}

// Version returns the current migration version and dirty flag.
func Version(dsn string) (uint, bool, error) {
	m, err := newMigrate(dsn)
	if err != nil {
		return 0, false, err
	}
	return m.Version()
}

// Force sets the migration version to v (use with caution).
func Force(dsn string, v int) error {
	m, err := newMigrate(dsn)
	if err != nil {
		return err
	}
	return m.Force(v)
}

func newMigrate(dsn string) (*migrate.Migrate, error) {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate instance: %w", err)
	}
	return m, nil
}
