package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/redhat-et/skillimage/internal/store"
	"github.com/redhat-et/skillimage/pkg/oci"
)

// ContentConfig holds registry connection settings for on-demand content retrieval.
type ContentConfig struct {
	RegistryURL   string
	SkipTLSVerify bool
}

// SkillsHandler provides HTTP handlers for skill listing and detail.
type SkillsHandler struct {
	store      *store.Store
	contentCfg ContentConfig
}

// NewSkillsHandler creates a handler backed by the given store.
func NewSkillsHandler(s *store.Store, cfg ContentConfig) *SkillsHandler {
	return &SkillsHandler{store: s, contentCfg: cfg}
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

// Content handles GET /api/v1/skills/{ns}/{name}/versions/{ver}/content.
// It pulls the skill layer from the registry on demand and returns SKILL.md.
func (h *SkillsHandler) Content(w http.ResponseWriter, r *http.Request, ns, name, ver string) {
	versions, err := h.store.GetVersions(ns, name)
	if err != nil || len(versions) == 0 {
		writeError(w, http.StatusNotFound, "skill not found", fmt.Errorf("no versions for %s/%s", ns, name))
		return
	}

	var skill *store.Skill
	for i := range versions {
		if versions[i].Version == ver {
			skill = &versions[i]
			break
		}
	}
	if skill == nil {
		writeError(w, http.StatusNotFound, "version not found", fmt.Errorf("version %s not found", ver))
		return
	}

	ref := fmt.Sprintf("%s/%s:%s", h.contentCfg.RegistryURL, skill.Repository, skill.Tag)
	tmpDir, err := os.MkdirTemp("", "skillctl-content-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	storeDir, err := os.MkdirTemp("", "skillctl-store-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	defer os.RemoveAll(storeDir)

	client, err := oci.NewClient(storeDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	_, err = client.Pull(r.Context(), ref, oci.PullOptions{
		OutputDir:     tmpDir,
		SkipTLSVerify: h.contentCfg.SkipTLSVerify,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "pull failed", err)
		return
	}

	content, err := findSkillMD(tmpDir)
	if err != nil {
		writeError(w, http.StatusNotFound, "SKILL.md not found", err)
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func findSkillMD(dir string) ([]byte, error) {
	var content []byte
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "SKILL.md" && !d.IsDir() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content = data
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, fmt.Errorf("SKILL.md not found in extracted content")
	}
	return content, nil
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
