// @title          we-sync API
// @version        1.0
// @description    API de gerenciamento financeiro — lançamentos de contas mensais.
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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/configs"
	"github.com/jonasjesusamerico/we-sync-api/internal/handler"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
	"github.com/jonasjesusamerico/we-sync-api/internal/purchase"
	"github.com/jonasjesusamerico/we-sync-api/internal/router"
	"github.com/jonasjesusamerico/we-sync-api/internal/telemetry"
)

func main() {
	slog.Info("Iniciando a aplicação...")

	// Configs
	properties, err := configs.LoadProperties()
	if err != nil {
		slog.Error("Erro ao carregar configurações", "error", err)
		os.Exit(1)
	}
	slog.Info("Configurações carregadas com sucesso")

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
		slog.Error("Erro ao inicializar telemetria", "error", err)
		os.Exit(1)
	}
	slog.Info("Telemetria inicializada",
		"service", properties.Telemetry.ServiceName,
		"otlp_endpoint", properties.Telemetry.OTLPEndpoint,
		"prometheus_port", properties.Telemetry.PrometheusPort,
	)

	// DB
	slog.Info("Conectando ao banco de dados...")
	db, err := configs.NewConnection(properties)
	if err != nil {
		slog.Error("Erro ao conectar ao banco", "error", err)
		os.Exit(1)
	}
	slog.Info("Conexão com banco estabelecida")

	// Purchase entries
	purchaseRepo := purchase.NewPurchaseRepository(db)
	purchaseSvc := purchase.NewPurchaseService(purchaseRepo)
	purchaseHandler := purchase.NewPurchaseHandler(purchaseSvc)
	authRepo := auth.NewAuthRepository(db)
	authSvc := auth.NewAuthService(
		authRepo,
		[]byte(properties.JWT.Secret),
		properties.JWT.AccessTokenDuration,
		properties.JWT.RefreshTokenDuration,
	)
	authHandler := auth.NewAuthHandler(authSvc)

	// Server setup
	handlers := router.Handlers{
		Health:   handler.NewHealthHandler(),
		Auth:     authHandler,
		Purchase: purchaseHandler,
	}

	server := &http.Server{
		Addr: ":" + properties.Server.Port,
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

	// Canal para capturar sinais do SO
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Rodar servidor em goroutine
	go func() {
		slog.Info("Servidor HTTP rodando", "port", properties.Server.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Erro no servidor HTTP", "error", err)
			os.Exit(1)
		}
	}()

	// Espera sinal de shutdown
	<-stop
	slog.Info("Shutdown iniciado...")

	// Context com timeout para shutdown — dá tempo às requests em voo terminarem.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Para de aceitar novas conexões e aguarda as em voo.
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Erro ao desligar servidor", "error", err)
	} else {
		slog.Info("Servidor desligado com sucesso")
	}

	// 2. Flush de traces e métricas pendentes.
	if err := telShutdown(ctx); err != nil {
		slog.Error("Erro ao fechar telemetria", "error", err)
	} else {
		slog.Info("Telemetria encerrada com sucesso")
	}

	// 3. Fecha conexões com o banco após garantir que nenhuma query está em voo.
	if err := db.Write.Close(); err != nil {
		slog.Error("Erro ao fechar DB Write", "error", err)
	}
	if err := db.Read.Close(); err != nil {
		slog.Error("Erro ao fechar DB Read", "error", err)
	}

	slog.Info("Aplicação finalizada")
}
