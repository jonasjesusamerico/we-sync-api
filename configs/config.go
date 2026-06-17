package configs

import (
	"fmt"
	"os"

	log "log/slog"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnvName              string
	SystemProperties        SystemProperties
	DatabaseWriteProperties DatabaseWriteProperties
	DatabaseReadProperties  DatabaseReadProperties
}

type SystemProperties struct {
	Port string
}

type DatabaseWriteProperties struct {
	DATABASE_WRITE_DSN string
}

type DatabaseReadProperties struct {
	DATABASE_READ_DSN string
}

func Load() *Config {
	log.Info("Carregando configurações...")
	// Apenas para carregar caso esteja executando localmente.
	_ = godotenv.Load()

	config := &Config{
		AppEnvName: getEnv("APP_ENV_NAME", "DEV", false),
		SystemProperties: SystemProperties{
			Port: getEnv("SERVER_PORT", "8080", true),
		},
		DatabaseWriteProperties: DatabaseWriteProperties{
			DATABASE_WRITE_DSN: getEnv("DATABASE_WRITE_DSN", "", true),
		},
		DatabaseReadProperties: DatabaseReadProperties{
			DATABASE_READ_DSN: getEnv("DATABASE_READ_DSN", "", true),
		},
	}

	log.Info("Configurações carregadas com sucesso!")
	return config
}

func getEnv(key, fallback string, required bool) string {
	value := os.Getenv(key)

	if value == "" {
		if required {
			log.Error(fmt.Sprintf("Propriedade não encontrada: %s", key))
			os.Exit(1)
		}
		return fallback
	}

	return value
}
