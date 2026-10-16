package configs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnvName              string
	EnablePprof             bool
	EnableSwagger           bool
	CORSAllowedOrigins      []string
	Server                  ServerProperties
	DatabaseWriteProperties DatabaseProperties
	DatabaseReadProperties  DatabaseProperties
	Logger                  LoggerProperties
	Telemetry               TelemetryProperties
	JWT                     JWTProperties
}

// JWTProperties holds the settings used to sign and expire authentication tokens.
type JWTProperties struct {
	Secret               string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
}

// TelemetryProperties holds OpenTelemetry configuration.
type TelemetryProperties struct {
	// ServiceName is the logical name reported to the OTel backend.
	ServiceName string
	// ServiceVersion is the deployed version string (e.g. "1.0.0").
	ServiceVersion string
	// OTLPEndpoint is the gRPC address of the OTel Collector (host:port).
	// Leave empty to disable OTLP export (Prometheus scrape still works).
	OTLPEndpoint string
	// PrometheusPort is the port where /metrics is exposed. Default: "9090".
	PrometheusPort string
}

type LoggerProperties struct {
	Level int
}

type ServerProperties struct {
	Port string
}

type DatabaseProperties struct {
	DATABASE_DSN                string
	DATABASE_MAX_OPEN_CONNS     int
	DATABASE_MAX_IDLE_CONNS     int
	DATABASE_CONN_MAX_LIFETIME  int
	DATABASE_CONN_MAX_IDLE_TIME int
}

func LoadProperties() (*Config, error) {
	// Loads .env only when running locally; missing file is not an error.
	_ = godotenv.Load()

	serverPort, err := getEnv("SERVER_PORT", "8080", false)
	if err != nil {
		return nil, err
	}

	enablePprof, err := getEnvAsBool("ENABLE_PPROF", "false", false)
	if err != nil {
		return nil, err
	}

	enableSwagger, err := getEnvAsBool("ENABLE_SWAGGER", "false", false)
	if err != nil {
		return nil, err
	}

	loggerLevel, err := getEnvAsInt("LOGGER_LEVEL", "0", false)
	if err != nil {
		return nil, err
	}

	writeDSN, err := getEnv("DATABASE_WRITE_DSN", "", true)
	if err != nil {
		return nil, err
	}
	writeMaxOpen, err := getEnvAsInt("DATABASE_WRITE_MAX_OPEN_CONNS", "", true)
	if err != nil {
		return nil, err
	}
	writeMaxIdle, err := getEnvAsInt("DATABASE_WRITE_MAX_IDLE_CONNS", "", true)
	if err != nil {
		return nil, err
	}
	writeMaxLifetime, err := getEnvAsInt("DATABASE_WRITE_CONN_MAX_LIFETIME", "", true)
	if err != nil {
		return nil, err
	}
	writeMaxIdleTime, err := getEnvAsInt("DATABASE_WRITE_CONN_MAX_IDLE_TIME", "", true)
	if err != nil {
		return nil, err
	}

	readDSN, err := getEnv("DATABASE_READ_DSN", "", true)
	if err != nil {
		return nil, err
	}
	readMaxOpen, err := getEnvAsInt("DATABASE_READ_MAX_OPEN_CONNS", "", true)
	if err != nil {
		return nil, err
	}
	readMaxIdle, err := getEnvAsInt("DATABASE_READ_MAX_IDLE_CONNS", "", true)
	if err != nil {
		return nil, err
	}
	readMaxLifetime, err := getEnvAsInt("DATABASE_READ_CONN_MAX_LIFETIME", "", true)
	if err != nil {
		return nil, err
	}
	readMaxIdleTime, err := getEnvAsInt("DATABASE_READ_CONN_MAX_IDLE_TIME", "", true)
	if err != nil {
		return nil, err
	}

	appEnvName, err := getEnv("APP_ENV_NAME", "DEV", false)
	if err != nil {
		return nil, err
	}

	corsAllowedOrigins, err := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173", false)
	if err != nil {
		return nil, err
	}

	// --- Telemetry (OTel) ---
	otelServiceName, err := getEnv("OTEL_SERVICE_NAME", "we-sync-api", false)
	if err != nil {
		return nil, err
	}
	otelServiceVersion, err := getEnv("OTEL_SERVICE_VERSION", "1.0.0", false)
	if err != nil {
		return nil, err
	}
	otelEndpoint, err := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "", false)
	if err != nil {
		return nil, err
	}
	otelPrometheusPort, err := getEnv("OTEL_PROMETHEUS_PORT", "9090", false)
	if err != nil {
		return nil, err
	}

	jwtSecret, err := getEnv("JWT_SECRET", "", true)
	if err != nil {
		return nil, err
	}
	if len([]byte(jwtSecret)) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	if jwtSecret == "local-development-jwt-secret-change-before-production-2026" {
		return nil, fmt.Errorf("JWT_SECRET must not use the default development secret")
	}

	jwtAccessTokenDuration, err := getEnvAsDuration("JWT_ACCESS_TOKEN_DURATION", "15m", false)
	if err != nil {
		return nil, err
	}
	if jwtAccessTokenDuration <= 0 || jwtAccessTokenDuration > 24*time.Hour {
		return nil, fmt.Errorf("JWT_ACCESS_TOKEN_DURATION must be positive and at most 24h")
	}
	jwtRefreshTokenDuration, err := getEnvAsDuration("JWT_REFRESH_TOKEN_DURATION", "168h", false)
	if err != nil {
		return nil, err
	}
	if jwtRefreshTokenDuration <= 0 || jwtRefreshTokenDuration > 30*24*time.Hour {
		return nil, fmt.Errorf("JWT_REFRESH_TOKEN_DURATION must be positive and at most 720h")
	}

	config := &Config{
		AppEnvName:         appEnvName,
		EnablePprof:        enablePprof,
		EnableSwagger:      enableSwagger,
		CORSAllowedOrigins: strings.Split(corsAllowedOrigins, ","),
		Server: ServerProperties{
			Port: serverPort,
		},
		DatabaseWriteProperties: DatabaseProperties{
			DATABASE_DSN:                writeDSN,
			DATABASE_MAX_OPEN_CONNS:     writeMaxOpen,
			DATABASE_MAX_IDLE_CONNS:     writeMaxIdle,
			DATABASE_CONN_MAX_LIFETIME:  writeMaxLifetime,
			DATABASE_CONN_MAX_IDLE_TIME: writeMaxIdleTime,
		},
		DatabaseReadProperties: DatabaseProperties{
			DATABASE_DSN:                readDSN,
			DATABASE_MAX_OPEN_CONNS:     readMaxOpen,
			DATABASE_MAX_IDLE_CONNS:     readMaxIdle,
			DATABASE_CONN_MAX_LIFETIME:  readMaxLifetime,
			DATABASE_CONN_MAX_IDLE_TIME: readMaxIdleTime,
		},
		Logger: LoggerProperties{
			// LevelDebug Level = -4
			// LevelInfo  Level = 0
			// LevelWarn  Level = 4
			// LevelError Level = 8
			Level: loggerLevel, // Default INFO
		},
		Telemetry: TelemetryProperties{
			ServiceName:    otelServiceName,
			ServiceVersion: otelServiceVersion,
			OTLPEndpoint:   otelEndpoint,
			PrometheusPort: otelPrometheusPort,
		},
		JWT: JWTProperties{
			Secret:               jwtSecret,
			AccessTokenDuration:  jwtAccessTokenDuration,
			RefreshTokenDuration: jwtRefreshTokenDuration,
		},
	}

	return config, nil
}

func getEnv(key, fallback string, required bool) (string, error) {
	value := os.Getenv(key)

	if value == "" {
		if required {
			return "", fmt.Errorf("required environment variable not set: %s", key)
		}
		return fallback, nil
	}

	return value, nil
}

func getEnvAsInt(key string, fallback string, required bool) (int, error) {
	value, err := getEnv(key, fallback, required)
	if err != nil {
		return 0, err
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid value for environment variable %s: %q (expected integer)", key, value)
	}

	return intValue, nil
}

func getEnvAsBool(key string, fallback string, required bool) (bool, error) {
	value, err := getEnv(key, fallback, required)
	if err != nil {
		return false, err
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid value for environment variable %s: %q (expected true/false)", key, value)
	}

	return boolValue, nil
}

func getEnvAsDuration(key string, fallback string, required bool) (time.Duration, error) {
	value, err := getEnv(key, fallback, required)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid value for environment variable %s: %q (expected duration)", key, value)
	}

	return duration, nil
}
