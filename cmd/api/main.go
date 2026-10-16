// @title          we-sync API
// @version        1.0
// @description    Personal finance API — monthly purchase entries.
//
// @host           localhost:8080
// @BasePath       /api/v1
//
// @contact.name   Jonas Jesus Americo
// @contact.url    https://github.com/jonasjesusamerico
//
// @schemes        http https
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/config"
	"github.com/jonasjesusamerico/we-sync-api/internal/database"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
	"github.com/jonasjesusamerico/we-sync-api/internal/purchase"
	"github.com/jonasjesusamerico/we-sync-api/internal/router"
	"github.com/jonasjesusamerico/we-sync-api/internal/telemetry"
)

func main() {
	slog.Info("Starting application")

	// Configs
	properties, err := config.LoadProperties()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("Configuration loaded")

	// Telemetry — must be initialised before any instrumented code runs.
	// The background context is intentional: OTel bootstrap uses it only
	// to connect exporters; request-scoped contexts are passed per operation.
	telShutdown, err := telemetry.Bootstrap(context.Background(), telemetry.Config{
		ServiceName:    properties.Telemetry.ServiceName,
		ServiceVersion: properties.Telemetry.ServiceVersion,
		Environment:    properties.AppEnvName,
		OTLPEndpoint:   properties.Telemetry.OTLPEndpoint,
		PrometheusPort: properties.Telemetry.PrometheusPort,
	})
	if err != nil {
		slog.Error("Failed to initialize telemetry", "error", err)
		os.Exit(1)
	}
	slog.Info("Telemetry initialized",
		"service", properties.Telemetry.ServiceName,
		"otlp_endpoint", properties.Telemetry.OTLPEndpoint,
		"prometheus_port", properties.Telemetry.PrometheusPort,
	)

	// DB
	slog.Info("Connecting to database")
	db, err := database.NewConnection(properties)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	slog.Info("Database connection established")

	// Purchase entries
	purchaseRepo := purchase.NewRepository(db)
	categoryRepo := category.NewRepository(db)
	purchaseSvc := purchase.NewService(purchaseRepo, categoryRepo)
	purchaseHandler := purchase.NewHandler(purchaseSvc)
	categorySvc := category.NewService(categoryRepo)
	categoryHandler := category.NewHandler(categorySvc)
	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(
		authRepo,
		[]byte(properties.JWT.Secret),
		properties.JWT.AccessTokenDuration,
		properties.JWT.RefreshTokenDuration,
	)
	authHandler := auth.NewHandler(authSvc)

	// Server setup
	handlers := router.Handlers{
		Auth:     authHandler,
		Purchase: purchaseHandler,
		Category: categoryHandler,
	}

	server := &http.Server{
		Addr: ":" + properties.Server.Port,
		// Timeouts protect against slow clients (e.g. slowloris) holding
		// connections open. WriteTimeout matches nginx proxy_read_timeout.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		Handler: router.New(
			handlers,
			logger.New(properties.Logger),
			properties.Telemetry.ServiceName,
			properties.EnablePprof,
			properties.EnableSwagger,
			properties.CORSAllowedOrigins,
			[]byte(properties.JWT.Secret),
		),
	}

	// Channel that receives OS shutdown signals.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Run the server in a goroutine.
	go func() {
		slog.Info("HTTP server listening", "port", properties.Server.Port)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for a shutdown signal.
	<-stop
	slog.Info("Shutdown started")

	// Shutdown context with timeout — gives in-flight requests time to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Stop accepting new connections and wait for in-flight ones.
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Failed to shut down server", "error", err)
	} else {
		slog.Info("Server shut down")
	}

	// 2. Flush pending traces and metrics.
	if err := telShutdown(ctx); err != nil {
		slog.Error("Failed to shut down telemetry", "error", err)
	} else {
		slog.Info("Telemetry shut down")
	}

	// 3. Close database pools once no query is in flight.
	if err := db.Write.Close(); err != nil {
		slog.Error("Failed to close write DB", "error", err)
	}
	if err := db.Read.Close(); err != nil {
		slog.Error("Failed to close read DB", "error", err)
	}

	slog.Info("Application stopped")
}
