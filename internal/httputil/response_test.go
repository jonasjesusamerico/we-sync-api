package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		write      func(http.ResponseWriter, string)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "unauthorized",
			write:      WriteUnauthorized,
			wantStatus: http.StatusUnauthorized,
			wantCode:   ErrCodeUnauthorized,
		},
		{
			name:       "forbidden",
			write:      WriteForbidden,
			wantStatus: http.StatusForbidden,
			wantCode:   ErrCodeForbidden,
		},
		{
			name:       "conflict",
			write:      WriteConflict,
			wantStatus: http.StatusConflict,
			wantCode:   ErrCodeConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			tt.write(recorder, "test message")

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}

			var response APIError
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", response.Code, tt.wantCode)
			}
			if response.Message != "test message" {
				t.Errorf("message = %q, want test message", response.Message)
			}
		})
	}
}

// TestWriteError_NewCodes pins the codes and literal values of the feature 006 contract.
func TestWriteError_NewCodes(t *testing.T) {
	tests := []struct {
		status   int
		code     string
		wantCode string
		message  string
	}{
		{http.StatusForbidden, ErrCodeInvalidPassword, "INVALID_PASSWORD", "invalid password"},
		{http.StatusConflict, ErrCodeDefaultNameConflict, "DEFAULT_NAME_CONFLICT", "name already used by a default category"},
		{http.StatusConflict, ErrCodeNameConflict, "NAME_CONFLICT", "name already used by another category"},
		{http.StatusConflict, ErrCodeArchivedNameConflict, "ARCHIVED_NAME_CONFLICT", "name already used by an archived category"},
	}

	for _, tt := range tests {
		t.Run(tt.wantCode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			WriteError(recorder, tt.status, tt.code, tt.message)

			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}
			var body map[string]string
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(body) != 2 || body["code"] != tt.wantCode || body["message"] != tt.message {
				t.Errorf("body = %v, want code %q and message %q", body, tt.wantCode, tt.message)
			}
		})
	}
}
