package main

import (
	"fmt"
	"log/slog"

	"github.com/jonasjesusamerico/we-sync-api/configs"
)

func main() {

	// Carrega as configurações
	var config = configs.Load()
	slog.Info("Configurações carregadas com sucesso!", "config", fmt.Sprintf("%+v", config))

	// Aqui será a inicializacao do banco de dados

	// Aqui será a inicialização do servidor HTTP

	// O servidor HTTP deve ser iniciado com as configurações carregadas

}
