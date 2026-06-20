package configs

import (
	"database/sql"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	Write *sql.DB
	Read  *sql.DB
}

func NewConnection(config *Config) (*DB, error) {
	slog.Info("Estabelecendo conexões com os bancos de dados de escrita...")
	writeDB, err := sql.Open("pgx", config.DatabaseWriteProperties.DATABASE_DSN)
	if err != nil {
		return nil, err
	}

	slog.Info("Configurando pools de conexões de escrita...")
	configurePool(writeDB, config.DatabaseWriteProperties)

	slog.Info("Verificando conexões com os bancos de dados de escrita...")
	if err := writeDB.Ping(); err != nil {
		_ = writeDB.Close()
		return nil, err
	}

	slog.Info("Estabelecendo conexões com os bancos de dados de leitura...")
	readDB, err := sql.Open("pgx", config.DatabaseReadProperties.DATABASE_DSN)
	if err != nil {
		return nil, err
	}

	slog.Info("Configurando pools de conexões de leitura...")
	configurePool(readDB, config.DatabaseReadProperties)

	slog.Info("Verificando conexões com os bancos de dados de leitura...")
	if err := readDB.Ping(); err != nil {
		_ = writeDB.Close()
		_ = readDB.Close()
		return nil, err
	}

	slog.Info("Todas as conexões com os bancos de dados estabelecidas com sucesso!")
	return &DB{
		Write: writeDB,
		Read:  readDB,
	}, nil
}

func configurePool(db *sql.DB, properties DatabaseProperties) {
	db.SetMaxOpenConns(properties.DATABASE_MAX_OPEN_CONNS)
	db.SetMaxIdleConns(properties.DATABASE_MAX_IDLE_CONNS)
	db.SetConnMaxLifetime(time.Duration(properties.DATABASE_CONN_MAX_LIFETIME) * time.Minute)
	db.SetConnMaxIdleTime(time.Duration(properties.DATABASE_CONN_MAX_IDLE_TIME) * time.Minute)
}
