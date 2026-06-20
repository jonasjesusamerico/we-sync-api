package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/configs"
	"github.com/jonasjesusamerico/we-sync-api/internal/handler"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
	"github.com/jonasjesusamerico/we-sync-api/internal/router"
)

func main() {
	slog.Info("Iniciando a aplicação...")

	// Configs
	properties := configs.LoadProperties()

	// DB
	slog.Info("Conectando ao banco de dados...")
	db, err := configs.NewConnection(properties)
	if err != nil {
		slog.Error("Erro ao conectar ao banco", "error", err)
		log.Fatal(err)
	}
	slog.Info("Conexão com banco estabelecida")

	// Server setup
	handlers := router.Handlers{
		Health: handler.NewHealthHandler(),
	}

	server := &http.Server{
		Addr:    ":" + properties.System.Port,
		Handler: router.New(handlers, logger.New(properties.Logger), properties.EnablePprof),
	}

	// Canal para capturar sinais do SO
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Rodar servidor em goroutine
	go func() {
		slog.Info("Servidor HTTP rodando", "port", properties.System.Port)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Erro no servidor HTTP", "error", err)
			log.Fatal(err)
		}
	}()

	// Espera sinal de shutdown
	<-stop
	slog.Info("Shutdown iniciado...")

	// Context com timeout para shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Desliga servidor com grace period
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Erro ao desligar servidor", "error", err)
	} else {
		slog.Info("Servidor desligado com sucesso")
	}

	// Fecha banco depois do shutdown do HTTP
	if err := db.Write.Close(); err != nil {
		slog.Error("Erro ao fechar DB Write", "error", err)
	}
	if err := db.Read.Close(); err != nil {
		slog.Error("Erro ao fechar DB Read", "error", err)
	}

	slog.Info("Aplicação finalizada")
}
