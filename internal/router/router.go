package router

import (
	"log/slog"
	"net/http"

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

	registerMiddlewares(r)

	r.Route("/", func(r chi.Router) {
		r.Get("/health-check", h.Health.Check)
	})

	r.Route("/api", func(r chi.Router) {
		registerV1(r, h)
	})

	return r
}

func registerMiddlewares(r *chi.Mux) {
	// r.Use(middleware.RequestID)
	// r.Use(middleware.Logger)
	// r.Use(middleware.Recovery)
	// r.Use(middleware.CORS)
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
