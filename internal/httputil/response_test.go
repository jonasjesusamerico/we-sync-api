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
