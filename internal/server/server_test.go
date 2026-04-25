package server_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/server"
	"github.com/redhat-et/skillimage/internal/store"
)

func TestRouterIntegration(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
		Version: "1.0.0", DisplayName: "Document Reviewer",
		Description: "Reviews docs", TagsJSON: `["review"]`,
	})

	router := server.NewRouter(db, func() {}, handler.ContentConfig{})

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/healthz", 200},
		{"GET", "/api/v1/skills", 200},
		{"GET", "/api/v1/skills?status=published", 200},
		{"GET", "/api/v1/skills?q=review", 200},
		{"GET", "/api/v1/skills/team1/doc-reviewer", 200},
		{"GET", "/api/v1/skills/team1/doc-reviewer/versions", 200},
		{"GET", "/api/v1/skills/team1/nonexistent", 404},
		{"POST", "/api/v1/sync", 202},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d; body: %s",
					w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestRouterPagination(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
		Version: "1.0.0", DisplayName: "Document Reviewer",
		Description: "Reviews docs", TagsJSON: `["review"]`,
	})

	router := server.NewRouter(db, func() {}, handler.ContentConfig{})

	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Total   int `json:"total"`
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Pagination.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Pagination.Total)
	}
	if resp.Pagination.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Pagination.Page)
	}
	if resp.Pagination.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", resp.Pagination.PerPage)
	}
}

func TestRouterHealthResponse(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	router := server.NewRouter(db, func() {}, handler.ContentConfig{})

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("health status = %q, want %q", resp["status"], "ok")
	}
}

func TestRouterFiltersByNamespace(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/skill-a", Tag: "1.0.0", Digest: "sha256:aaa",
		Name: "skill-a", Namespace: "team1", Status: "published",
	})
	_ = db.UpsertSkill(store.Skill{
		Repository: "team2/skill-b", Tag: "1.0.0", Digest: "sha256:bbb",
		Name: "skill-b", Namespace: "team2", Status: "published",
	})

	router := server.NewRouter(db, func() {}, handler.ContentConfig{})

	req := httptest.NewRequest("GET", "/api/v1/skills?namespace=team1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Pagination.Total != 1 {
		t.Errorf("total = %d, want 1 (filtered by namespace=team1)", resp.Pagination.Total)
	}
}
