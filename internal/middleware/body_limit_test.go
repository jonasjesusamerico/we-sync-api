package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodySize_RejectsOversizedBody(t *testing.T) {
	var readErr error
	handler := MaxBodySize(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 9)))
	handler.ServeHTTP(httptest.NewRecorder(), request)

	var maxErr *http.MaxBytesError
	if !errors.As(readErr, &maxErr) {
		t.Fatalf("read error = %v, want *http.MaxBytesError", readErr)
	}
}

func TestMaxBodySize_AllowsBodyWithinLimit(t *testing.T) {
	var body []byte
	var readErr error
	handler := MaxBodySize(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr = io.ReadAll(r.Body)
	}))
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345678"))
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if readErr != nil || string(body) != "12345678" {
		t.Fatalf("ReadAll() = %q, %v", body, readErr)
	}
}
