package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

func setupStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestListSkillsHandler(t *testing.T) {
	db := setupStore(t)
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
		Version: "1.0.0", Description: "Reviews docs",
	})

	h := handler.NewSkillsHandler(db, handler.ContentConfig{})
	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

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
	if len(resp.Data) != 1 {
		t.Errorf("got %d skills, want 1", len(resp.Data))
	}
	if resp.Pagination.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Pagination.Total)
	}
}

func TestListSkillsEmpty(t *testing.T) {
	db := setupStore(t)
	h := handler.NewSkillsHandler(db, handler.ContentConfig{})
	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data array, got %d", len(resp.Data))
	}
}

func TestListSkillsWithFilters(t *testing.T) {
	db := setupStore(t)
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/a", Tag: "1.0.0", Digest: "sha256:a",
		Name: "a", Namespace: "team1", Status: "published",
	})
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/b", Tag: "2.0.0-draft", Digest: "sha256:b",
		Name: "b", Namespace: "team1", Status: "draft",
	})

	h := handler.NewSkillsHandler(db, handler.ContentConfig{})

	req := httptest.NewRequest("GET", "/api/v1/skills?status=published", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("got %d published skills, want 1", len(resp.Data))
	}
}

func TestGetByNamespace(t *testing.T) {
	db := setupStore(t)
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
	})

	h := handler.NewSkillsHandler(db, handler.ContentConfig{})
	req := httptest.NewRequest("GET", "/api/v1/skills/team1/doc-reviewer", nil)
	w := httptest.NewRecorder()

	h.GetByNamespace(w, req, "team1", "doc-reviewer")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestGetByNamespaceNotFound(t *testing.T) {
	db := setupStore(t)
	h := handler.NewSkillsHandler(db, handler.ContentConfig{})
	req := httptest.NewRequest("GET", "/api/v1/skills/team1/nonexistent", nil)
	w := httptest.NewRecorder()

	h.GetByNamespace(w, req, "team1", "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestVersions(t *testing.T) {
	db := setupStore(t)
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0-draft",
		Digest: "sha256:aaa", Name: "doc-reviewer",
		Namespace: "team1", Version: "1.0.0", Status: "draft",
	})
	_ = db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:bbb", Name: "doc-reviewer",
		Namespace: "team1", Version: "1.0.0", Status: "published",
	})

	h := handler.NewSkillsHandler(db, handler.ContentConfig{})
	req := httptest.NewRequest("GET", "/api/v1/skills/team1/doc-reviewer/versions", nil)
	w := httptest.NewRecorder()

	h.Versions(w, req, "team1", "doc-reviewer")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("got %d versions, want 2", len(resp.Data))
	}
}
