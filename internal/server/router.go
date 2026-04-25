package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

// NewRouter builds the chi router with all API routes.
func NewRouter(db *store.Store, syncFn func()) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	skills := handler.NewSkillsHandler(db)
	syncH := handler.NewSyncHandler(syncFn)
	healthH := handler.NewHealthHandler()

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/skills", skills.List)
		r.Get("/skills/{ns}/{name}", func(w http.ResponseWriter, r *http.Request) {
			skills.GetByNamespace(w, r, chi.URLParam(r, "ns"), chi.URLParam(r, "name"))
		})
		r.Get("/skills/{ns}/{name}/versions", func(w http.ResponseWriter, r *http.Request) {
			skills.Versions(w, r, chi.URLParam(r, "ns"), chi.URLParam(r, "name"))
		})
		r.Post("/sync", syncH.Trigger)
	})

	r.Get("/healthz", healthH.Check)

	return r
}
