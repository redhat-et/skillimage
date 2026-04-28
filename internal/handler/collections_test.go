package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/internal/handler"
	"github.com/redhat-et/skillimage/internal/store"
)

func TestCollectionsList(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/org/collections/hr",
		Tag:         "1.0.0",
		Digest:      "sha256:abc",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR collection",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatal(err)
	}

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Data []store.Collection `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "hr-skills" {
		t.Errorf("name = %q, want %q", resp.Data[0].Name, "hr-skills")
	}
}

func TestCollectionsGet(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	col := store.Collection{
		Repository:  "quay.io/org/collections/hr",
		Tag:         "1.0.0",
		Digest:      "sha256:abc",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR collection",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatal(err)
	}

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections/hr-skills", nil)
	w := httptest.NewRecorder()
	h.Get(w, req, "hr-skills")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestCollectionsGetNotFound(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	h := handler.NewCollectionsHandler(db)
	req := httptest.NewRequest("GET", "/api/v1/collections/missing", nil)
	w := httptest.NewRecorder()
	h.Get(w, req, "missing")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
