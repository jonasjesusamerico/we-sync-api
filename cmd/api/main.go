package main

import (
	"log"
	"log/slog"
	"net/http"

	"github.com/jonasjesusamerico/we-sync-api/configs"
	"github.com/jonasjesusamerico/we-sync-api/internal/handler"
	"github.com/jonasjesusamerico/we-sync-api/internal/router"
)

func main() {
	slog.Info("Iniciando a aplicação...")

	// Carrega as configurações
	var configProperties = configs.LoadProperties()

	// Aqui será a inicializacao do banco de dados
	slog.Info("Conectando ao banco de dados...")
	db, err := configs.NewConnection(configProperties)

	db.Write.Close()
	db.Read.Close()

	if err != nil {
		slog.Error("Erro ao conectar ao banco de dados", "error", err)
		log.Fatal(err)
	}
	slog.Info("Conexão com o banco de dados estabelecida com sucesso!")
	// Aqui será a inicialização do servidor HTTP

	healthHandler := handler.NewHealthHandler()

	handlers := router.Handlers{
		Health: healthHandler,
	}

	httpServer := &http.Server{
		Addr:    ":" + configProperties.SystemProperties.Port,
		Handler: router.New(handlers),
	}
	httpServer.SetKeepAlivesEnabled(true)

	slog.Info("Servidor HTTP iniciado na porta " + configProperties.SystemProperties.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Erro ao iniciar o servidor HTTP", "error", err)
		log.Fatal(err)
	}
	// O servidor HTTP deve ser iniciado com as configurações carregadas

}
