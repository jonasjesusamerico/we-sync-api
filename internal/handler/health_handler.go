package handler

import (
	"encoding/json"
	"net/http"

	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	log := logger.FromContext(
		r.Context(),
	)

	log.Info(
		"request received",
	)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
