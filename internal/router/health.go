package router

import (
	"net/http"

	"github.com/jonasjesusamerico/we-sync-api/internal/httputil"
	"github.com/jonasjesusamerico/we-sync-api/internal/logger"
)

// healthCheck is a liveness probe: it reports that the process serves HTTP
// and intentionally does not touch the database.
func healthCheck(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	log.Info("request received")

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
