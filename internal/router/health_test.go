package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthCheck_ReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	healthCheck(rec, httptest.NewRequest(http.MethodGet, "/health-check", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q, want it to contain status ok", rec.Body.String())
	}
}
