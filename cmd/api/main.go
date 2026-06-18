package main

import (
	"log"
	"log/slog"

	"github.com/jonasjesusamerico/we-sync-api/configs"
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

	// O servidor HTTP deve ser iniciado com as configurações carregadas

}
