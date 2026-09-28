package handler

import (
	"net/http"

	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	log.Info("request received")

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
