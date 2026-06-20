package configs

import (
	"fmt"
	"os"
	"strconv"

	"log/slog"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnvName              string
	EnablePprof             bool
	System                  SystemProperties
	DatabaseWriteProperties DatabaseProperties
	DatabaseReadProperties  DatabaseProperties
	Logger                  LoggerProperties
}

type LoggerProperties struct {
	Level int
}

type SystemProperties struct {
	Port string
}

type DatabaseProperties struct {
	DATABASE_DSN                string
	DATABASE_MAX_OPEN_CONNS     int
	DATABASE_MAX_IDLE_CONNS     int
	DATABASE_CONN_MAX_LIFETIME  int
	DATABASE_CONN_MAX_IDLE_TIME int
}

func LoadProperties() *Config {
	slog.Info("Carregando configurações...")
	// Apenas para carregar caso esteja executando localmente.
	_ = godotenv.Load()

	config := &Config{
		AppEnvName: getEnv("APP_ENV_NAME", "DEV", false),
		EnablePprof: getEnvAsBool("ENABLE_PPROF", "false", false),
		System: SystemProperties{
			Port: getEnv("SERVER_PORT", "8080", true),
		},
		DatabaseWriteProperties: DatabaseProperties{
			DATABASE_DSN:                getEnv("DATABASE_WRITE_DSN", "", true),
			DATABASE_MAX_OPEN_CONNS:     getEnvAsInt("DATABASE_WRITE_MAX_OPEN_CONNS", "", true),
			DATABASE_MAX_IDLE_CONNS:     getEnvAsInt("DATABASE_WRITE_MAX_IDLE_CONNS", "", true),
			DATABASE_CONN_MAX_LIFETIME:  getEnvAsInt("DATABASE_WRITE_CONN_MAX_LIFETIME", "", true),
			DATABASE_CONN_MAX_IDLE_TIME: getEnvAsInt("DATABASE_WRITE_CONN_MAX_IDLE_TIME", "", true),
		},
		DatabaseReadProperties: DatabaseProperties{
			DATABASE_DSN:                getEnv("DATABASE_READ_DSN", "", true),
			DATABASE_MAX_OPEN_CONNS:     getEnvAsInt("DATABASE_READ_MAX_OPEN_CONNS", "", true),
			DATABASE_MAX_IDLE_CONNS:     getEnvAsInt("DATABASE_READ_MAX_IDLE_CONNS", "", true),
			DATABASE_CONN_MAX_LIFETIME:  getEnvAsInt("DATABASE_READ_CONN_MAX_LIFETIME", "", true),
			DATABASE_CONN_MAX_IDLE_TIME: getEnvAsInt("DATABASE_READ_CONN_MAX_IDLE_TIME", "", true),
		},
		Logger: LoggerProperties{
			// LevelDebug Level = -4
			// LevelInfo  Level = 0
			// LevelWarn  Level = 4
			// LevelError Level = 8
			Level: getEnvAsInt("LOGGER_LEVEL", "0", false), // Default INFO

		},
	}

	slog.Info("Configurações carregadas com sucesso!")
	return config
}

func getEnv(key, fallback string, required bool) string {
	value := os.Getenv(key)

	if value == "" {
		if required {
			slog.Error(fmt.Sprintf("Propriedade não encontrada: %s", key))
			os.Exit(1)
		}
		return fallback
	}

	return value
}

func getEnvAsInt(key string, fallback string, required bool) int {
	value := getEnv(key, fallback, required)
	intValue, err := strconv.Atoi(value)
	if err != nil {
		slog.Error(fmt.Sprintf("Valor inválido para a propriedade %s: %s", key, value))
		os.Exit(1)
	}
	return intValue
}

func getEnvAsBool(key string, fallback string, required bool) bool {
	value := getEnv(key, fallback, required)
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		slog.Error(fmt.Sprintf("Valor inválido para a propriedade %s: %s", key, value))
		os.Exit(1)
	}
	return boolValue
}
