package router

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/jonasjesusamerico/we-sync-api/internal/auth"
	"github.com/jonasjesusamerico/we-sync-api/internal/category"
	"github.com/jonasjesusamerico/we-sync-api/internal/handler"
	"github.com/jonasjesusamerico/we-sync-api/internal/middleware"
	"github.com/jonasjesusamerico/we-sync-api/internal/purchase"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "github.com/jonasjesusamerico/we-sync-api/docs/swagger"
)

// maxRequestBodyBytes caps every request body; JSON payloads of this API are small.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

// Handlers groups all domain handlers injected into the router.
type Handlers struct {
	Health   *handler.HealthHandler
	Auth     *auth.AuthHandler
	Purchase *purchase.PurchaseHandler
	Category *category.CategoryHandler
}

// New builds and returns the root HTTP handler.
// serviceName is forwarded to otelhttp to tag all spans with the correct service name.
func New(
	h Handlers,
	baseLogger *slog.Logger,
	serviceName string,
	enablePprof bool,
	enableSwagger bool,
	corsAllowedOrigins []string,
	jwtSecret []byte,
) http.Handler {
	r := chi.NewRouter()

	// otelhttp must be the outermost middleware so it wraps the full
	// request lifecycle, including all inner middlewares and handlers.
	// It automatically creates a server span, propagates W3C TraceContext
	// from incoming headers, and records http.server.* metrics.
	r.Use(otelhttp.NewMiddleware(serviceName))

	r.Use(middleware.Logging(baseLogger))
	r.Use(middleware.CORS(corsAllowedOrigins))
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.MaxBodySize(maxRequestBodyBytes))

	if enablePprof {
		registerPprof(r)
	}

	if enableSwagger {
		registerSwagger(r)
	}

	r.Route("/", func(r chi.Router) {
		r.Get("/health-check", h.Health.Check)
	})

	r.Route("/api", func(r chi.Router) {
		registerV1(r, h, jwtSecret)
	})

	return r
}

func registerV1(r chi.Router, h Handlers, jwtSecret []byte) {
	r.Route("/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", h.Auth.Register)
			r.Post("/login", h.Auth.Login)
			r.Post("/refresh", h.Auth.Refresh)
			r.Post("/logout", h.Auth.Logout)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTWithConfig(jwtSecret, auth.JWTIssuer, auth.JWTAudience, 24*time.Hour))
			r.Route("/purchases", func(r chi.Router) {
				r.Get("/", h.Purchase.List)
				r.Post("/", h.Purchase.Create)
				r.Get("/{id}", h.Purchase.GetByID)
				r.Put("/{id}", h.Purchase.Update)
				r.Delete("/{id}", h.Purchase.Delete)
			})
			r.Route("/categories", func(r chi.Router) {
				r.Get("/", h.Category.List)
				r.Post("/", h.Category.Create)
				r.Put("/{id}", h.Category.Update)
				r.Delete("/{id}", h.Category.Delete)
			})
		})
	})
}

func registerSwagger(r chi.Router) {
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))
}

func registerPprof(r chi.Router) {
	r.Route("/debug/pprof", func(r chi.Router) {
		r.Get("/", pprof.Index)
		r.Get("/cmdline", pprof.Cmdline)
		r.Get("/profile", pprof.Profile)
		r.Get("/symbol", pprof.Symbol)
		r.Get("/trace", pprof.Trace)

		r.Handle("/allocs", pprof.Handler("allocs"))
		r.Handle("/block", pprof.Handler("block"))
		r.Handle("/goroutine", pprof.Handler("goroutine"))
		r.Handle("/heap", pprof.Handler("heap"))
		r.Handle("/mutex", pprof.Handler("mutex"))
		r.Handle("/threadcreate", pprof.Handler("threadcreate"))
	})
}
