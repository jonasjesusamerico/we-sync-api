package database

import (
	"database/sql"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type Migrator struct {
	writeDB *sql.DB
}

func NewMigrator(writeDB *sql.DB) *Migrator {
	return &Migrator{writeDB: writeDB}
}

func (m *Migrator) Up() error {
	slog.Info("Iniciando migrações...")
	driver, err := postgres.WithInstance(m.writeDB, &postgres.Config{})
	mg, err := migrate.NewWithDatabaseInstance(
		"file:///migrations",
		"postgres", driver)
	mg.Up()

	slog.Info("Migrações criadas")
	if err != nil {
		slog.Error("Erro ao criar migrações", "error", err)
		return err
	}

	slog.Info("Aplicando migrações...")

	err = mg.Up()
	slog.Info("Migrações aplicadas")
	if err != nil && err != migrate.ErrNoChange {
		slog.Error("Erro ao aplicar migrações", "error", err)
		return err
	}
	slog.Info("Nenhuma mudança necessária")
	return nil
}

func (m *Migrator) Down() error {
	driver, err := postgres.WithInstance(m.writeDB, &postgres.Config{})
	mg, err := migrate.NewWithDatabaseInstance(
		"file:///migrations",
		"postgres", driver)

	if err != nil {
		return err
	}

	return mg.Down()
}
