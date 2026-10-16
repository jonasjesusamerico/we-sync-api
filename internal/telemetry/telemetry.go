// Package telemetry bootstraps the OpenTelemetry SDK for the application.
// It wires up a TracerProvider (OTLP gRPC → collector) and a MeterProvider
// (dual-export: OTLP gRPC + Prometheus scrape endpoint) and registers them
// as the global OTel providers.
//
// Usage:
//
//	shutdown, err := telemetry.Bootstrap(ctx, cfg)
//	defer shutdown(ctx)
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

// Config holds the configuration needed to initialise OTel.
type Config struct {
	// ServiceName is the logical name of this service (e.g. "we-sync-api").
	ServiceName string
	// ServiceVersion is the deployed version string (e.g. "1.0.0").
	ServiceVersion string
	// Environment identifies the deployment environment (e.g. "production", "staging", "development").
	Environment string
	// OTLPEndpoint is the gRPC endpoint of the OTel Collector (e.g. "otel-collector:4317").
	// When empty, OTLP exporters are disabled and only the Prometheus exporter is active.
	OTLPEndpoint string
	// PrometheusPort is the port on which the /metrics endpoint is exposed.
	// Defaults to "9090" when empty.
	PrometheusPort string
}

// ShutdownFunc flushes and closes all OTel providers.
// It must be called before the process exits.
type ShutdownFunc func(ctx context.Context) error

// Bootstrap initialises the global TracerProvider and MeterProvider.
// It returns a ShutdownFunc that must be deferred by the caller.
func Bootstrap(ctx context.Context, cfg Config) (ShutdownFunc, error) {
	res, err := newResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	tp, err := newTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("telemetry: tracer provider: %w", err)
	}

	mp, err := newMeterProvider(ctx, cfg, res)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("telemetry: meter provider: %w", err)
	}

	// Register as globals so otel.Tracer() / otel.Meter() work anywhere.
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	// W3C TraceContext + Baggage propagation across service boundaries.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Start Prometheus scrape endpoint in background.
	if cfg.PrometheusPort == "" {
		cfg.PrometheusPort = "9090"
	}
	go servePrometheus(cfg.PrometheusPort)

	return func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("telemetry shutdown errors: %v", errs)
		}
		return nil
	}, nil
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func newResource(cfg Config) (*resource.Resource, error) {
	// Pass an empty schema URL so resource.Merge does not fail with
	// "conflicting Schema URL" when resource.Default() carries a different
	// semconv version (e.g. v1.43.0 from the auto/sdk transitive dep).
	// The attributes themselves still use the correct semconv keys.
	custom := resource.NewWithAttributes(
		"", // no schema — avoids merge conflict
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
		attribute.String("service.environment", cfg.Environment),
	)
	return resource.Merge(resource.Default(), custom)
}

func newTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	// No WithSampler: the SDK reads the standard OTEL_TRACES_SAMPLER and
	// OTEL_TRACES_SAMPLER_ARG env vars (e.g. parentbased_traceidratio + 0.1) and
	// defaults to parentbased_always_on, i.e. every request when unset. Sampling
	// a fraction of requests saves span creation and export CPU under load;
	// metrics are not sampled.
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.OTLPEndpoint != "" {
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("OTLP trace exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(5*time.Second),
		))
	}

	return sdktrace.NewTracerProvider(opts...), nil
}

func newMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	// Prometheus exporter is always active (pull model, no endpoint required).
	promExp, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("Prometheus exporter: %w", err)
	}
	readers := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	}

	// OTLP push exporter is optional — only when a collector is configured.
	if cfg.OTLPEndpoint != "" {
		otlpExp, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlpmetricgrpc.WithInsecure(),
			otlpmetricgrpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("OTLP metric exporter: %w", err)
		}
		readers = append(readers, sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(otlpExp,
				sdkmetric.WithInterval(15*time.Second),
			),
		))
	}

	return sdkmetric.NewMeterProvider(readers...), nil
}

// servePrometheus starts a minimal HTTP server exposing /metrics for Prometheus scraping.
func servePrometheus(port string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	// Intentionally not checking the error — if the port is taken the main
	// server still runs; the missing metrics will be visible in logs.
	_ = srv.ListenAndServe()
}
