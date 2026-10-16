package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLogging_TraceIDOnlyForSampledTraces(t *testing.T) {
	tests := []struct {
		name      string
		flags     trace.TraceFlags
		wantTrace bool
	}{
		{"sampled", trace.FlagsSampled, true},
		{"not sampled", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    trace.TraceID{1},
				SpanID:     trace.SpanID{1},
				TraceFlags: tt.flags,
			})
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request = request.WithContext(trace.ContextWithSpanContext(request.Context(), sc))

			var out bytes.Buffer
			handler := Logging(slog.New(slog.NewJSONHandler(&out, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			handler.ServeHTTP(httptest.NewRecorder(), request)

			if got := strings.Contains(out.String(), `"trace_id"`); got != tt.wantTrace {
				t.Fatalf("log has trace_id = %t, want %t; log: %s", got, tt.wantTrace, out.String())
			}
			if !strings.Contains(out.String(), `"request_id"`) {
				t.Fatalf("log is missing request_id: %s", out.String())
			}
		})
	}
}
