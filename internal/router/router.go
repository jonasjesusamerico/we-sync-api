package router

import (
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/go-chi/chi/v5"
	"github.com/jonasjesusamerico/we-sync-api/internal/handler"
	"github.com/jonasjesusamerico/we-sync-api/internal/middleware"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type Handlers struct {
	Health *handler.HealthHandler
}

func New(h Handlers, baseLogger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logging(baseLogger))
	r.Use(middleware.CORS)
	r.Use(chimiddleware.Recoverer)

	registerPprof(r)

	r.Route("/", func(r chi.Router) {
		r.Get("/health-check", h.Health.Check)
	})

	r.Route("/api", func(r chi.Router) {
		registerV1(r, h)
	})

	return r
}

func registerV1(r chi.Router, h Handlers) {
	r.Route("/v1", func(r chi.Router) {

		// r.Get("/health", h.Health.Check)

		// r.Route("/auth", func(r chi.Router) {
		// 	r.Post("/login", h.Auth.Login)
		// 	r.Post("/register", h.Auth.Register)
		// })

		// r.Group(func(r chi.Router) {
		// 	r.Use(middleware.JWT)

		// 	r.Route("/lancamentos", func(r chi.Router) {
		// 		r.Get("/", h.Lancamento.List)
		// 		r.Post("/", h.Lancamento.Create)
		// 		r.Get("/{id}", h.Lancamento.GetByID)
		// 		r.Put("/{id}", h.Lancamento.Update)
		// 		r.Delete("/{id}", h.Lancamento.Delete)
		// 	})
		// })
	})
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
