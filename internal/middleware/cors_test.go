package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const nextBody = "next called"

func TestCORS(t *testing.T) {
	// One handler instance shared by all parallel subtests: under -race this also covers
	// concurrent reads of the allowlist. next reports that it ran through the response, so no
	// shared state is written by the subtests.
	handler := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(nextBody))
	}))

	tests := []struct {
		name            string
		method          string
		headers         map[string]string
		wantStatus      int
		wantNext        bool
		wantAllowOrigin string // empty means the header must be absent
		wantMaxAge      string // empty means the header must be absent
		wantPreflight   bool   // check Allow-Methods, Allow-Headers and Vary
	}{
		{
			name:   "preflight_allowed_origin_sets_max_age",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         "http://localhost:3000",
				"Access-Control-Request-Method":  "GET",
				"Access-Control-Request-Headers": "authorization",
			},
			wantStatus:      http.StatusNoContent,
			wantAllowOrigin: "http://localhost:3000",
			wantMaxAge:      "600",
			wantPreflight:   true,
		},
		{
			name:   "preflight_disallowed_origin_no_cors_grant",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "http://evil.example",
				"Access-Control-Request-Method": "GET",
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "preflight_without_origin",
			method:     http.MethodOptions,
			headers:    map[string]string{"Access-Control-Request-Method": "GET"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "origin_match_is_exact/trailing_slash",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "http://localhost:3000/",
				"Access-Control-Request-Method": "GET",
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "origin_match_is_exact/uppercase",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "HTTP://LOCALHOST:3000",
				"Access-Control-Request-Method": "GET",
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:            "simple_request_allowed_origin_no_max_age",
			method:          http.MethodGet,
			headers:         map[string]string{"Origin": "http://localhost:3000"},
			wantStatus:      http.StatusOK,
			wantNext:        true,
			wantAllowOrigin: "http://localhost:3000",
		},
		{
			name:       "simple_request_disallowed_origin",
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "http://evil.example"},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, "/api/v1/purchases", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if gotNext := rec.Body.String() == nextBody; gotNext != tt.wantNext {
				t.Errorf("next called = %v, want %v", gotNext, tt.wantNext)
			}
			assertHeader(t, rec, "Access-Control-Allow-Origin", tt.wantAllowOrigin)
			assertHeader(t, rec, "Access-Control-Max-Age", tt.wantMaxAge)

			if tt.wantPreflight {
				assertHeader(t, rec, "Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				assertHeader(t, rec, "Access-Control-Allow-Headers", "Authorization, Content-Type")
				assertHeader(t, rec, "Vary", "Origin")
			}
		})
	}
}

// assertHeader checks that the header has exactly the wanted value, or is absent when want is empty.
func assertHeader(t *testing.T, rec *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	got := rec.Header().Values(name)
	if want == "" {
		if len(got) != 0 {
			t.Errorf("header %s = %q, want absent", name, got)
		}
		return
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("header %s = %q, want %q", name, got, want)
	}
}
