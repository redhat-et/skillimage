package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

// NewRouter builds the chi router with all API routes.
func NewRouter(db *store.Store, syncFn func(), contentCfg handler.ContentConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	skills := handler.NewSkillsHandler(db, contentCfg)
	syncH := handler.NewSyncHandler(syncFn)
	healthH := handler.NewHealthHandler()

	r.Route("/api/v1", func(r chi.Router) {
		// Default to JSON for API endpoints; handlers that return other
		// content types (e.g. Content returns text/markdown) override this.
		r.Use(middleware.SetHeader("Content-Type", "application/json"))

		r.Get("/skills", skills.List)
		r.Get("/skills/{ns}/{name}", func(w http.ResponseWriter, r *http.Request) {
			skills.GetByNamespace(w, r, chi.URLParam(r, "ns"), chi.URLParam(r, "name"))
		})
		r.Get("/skills/{ns}/{name}/versions", func(w http.ResponseWriter, r *http.Request) {
			skills.Versions(w, r, chi.URLParam(r, "ns"), chi.URLParam(r, "name"))
		})
		r.With(middleware.Timeout(30*time.Second)).Get("/skills/{ns}/{name}/versions/{ver}/content", func(w http.ResponseWriter, r *http.Request) {
			skills.Content(w, r, chi.URLParam(r, "ns"), chi.URLParam(r, "name"), chi.URLParam(r, "ver"))
		})
		r.Post("/sync", syncH.Trigger)
	})

	r.Get("/healthz", healthH.Check)

	return r
}
