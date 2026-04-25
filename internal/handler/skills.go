package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/redhat-et/skillimage/internal/store"
)

// SkillsHandler provides HTTP handlers for skill listing and detail.
type SkillsHandler struct {
	store *store.Store
}

// NewSkillsHandler creates a handler backed by the given store.
func NewSkillsHandler(s *store.Store) *SkillsHandler {
	return &SkillsHandler{store: s}
}

type envelope struct {
	Data       any         `json:"data"`
	Pagination *pagination `json:"pagination,omitempty"`
}

type pagination struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// List handles GET /api/v1/skills with query parameters for filtering.
func (h *SkillsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	filter := store.ListFilter{
		Query:         q.Get("q"),
		Tags:          tags,
		Status:        q.Get("status"),
		Namespace:     q.Get("namespace"),
		Compatibility: q.Get("compatibility"),
		Page:          page,
		PerPage:       perPage,
	}

	skills, err := h.store.ListSkills(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skills", err)
		return
	}

	total, err := h.store.CountSkills(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "counting skills", err)
		return
	}

	if skills == nil {
		skills = []store.Skill{}
	}

	writeJSON(w, http.StatusOK, envelope{
		Data: skills,
		Pagination: &pagination{
			Total:   total,
			Page:    page,
			PerPage: perPage,
		},
	})
}

// GetByNamespace handles GET /api/v1/skills/{ns}/{name}.
func (h *SkillsHandler) GetByNamespace(w http.ResponseWriter, r *http.Request, ns, name string) {
	skill, err := h.store.GetSkill(ns, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "skill not found", err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: skill})
}

// Versions handles GET /api/v1/skills/{ns}/{name}/versions.
func (h *SkillsHandler) Versions(w http.ResponseWriter, r *http.Request, ns, name string) {
	versions, err := h.store.GetVersions(ns, name)
	if err != nil || len(versions) == 0 {
		writeError(w, http.StatusNotFound, "skill not found", err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: versions})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, title string, err error) {
	detail := title
	if err != nil {
		detail = err.Error()
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": detail,
	})
}
