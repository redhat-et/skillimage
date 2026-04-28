package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/redhat-et/skillimage/internal/store"
)

func TestNewStore(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills on empty db: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestUpsertAndList(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	skill := store.Skill{
		Repository:  "team1/doc-reviewer",
		Tag:         "1.0.0-draft",
		Digest:      "sha256:abc123",
		Name:        "doc-reviewer",
		Namespace:   "team1",
		Version:     "1.0.0",
		Status:      "draft",
		DisplayName: "Document Reviewer",
		Description: "Reviews docs",
		TagsJSON:    `["review","docs"]`,
		WordCount:   100,
	}
	if err := db.UpsertSkill(skill); err != nil {
		t.Fatalf("UpsertSkill: %v", err)
	}

	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "doc-reviewer" {
		t.Errorf("name = %q, want %q", skills[0].Name, "doc-reviewer")
	}
}

func TestUpsertUpdatesExisting(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	skill := store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0-draft",
		Digest: "sha256:aaa", Name: "doc-reviewer",
		Namespace: "team1", Description: "Old description",
	}
	if err := db.UpsertSkill(skill); err != nil {
		t.Fatalf("UpsertSkill (first): %v", err)
	}

	skill.Digest = "sha256:bbb"
	skill.Description = "New description"
	if err := db.UpsertSkill(skill); err != nil {
		t.Fatalf("UpsertSkill (second): %v", err)
	}

	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill after upsert, got %d", len(skills))
	}
	if skills[0].Digest != "sha256:bbb" {
		t.Errorf("digest = %q, want %q", skills[0].Digest, "sha256:bbb")
	}
	if skills[0].Description != "New description" {
		t.Errorf("description = %q, want %q", skills[0].Description, "New description")
	}
}

func TestListSkillsFilters(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	skills := []store.Skill{
		{Repository: "team1/skill-a", Tag: "1.0.0", Digest: "sha256:aaa", Name: "skill-a", Namespace: "team1", Status: "published", Description: "Kubernetes debugging", TagsJSON: `["kubernetes"]`},
		{Repository: "team1/skill-b", Tag: "2.0.0-draft", Digest: "sha256:bbb", Name: "skill-b", Namespace: "team1", Status: "draft", Description: "Python linting", TagsJSON: `["python"]`},
		{Repository: "team2/skill-c", Tag: "1.0.0", Digest: "sha256:ccc", Name: "skill-c", Namespace: "team2", Status: "published", Description: "Go testing", TagsJSON: `["go","testing"]`},
	}
	for _, sk := range skills {
		if err := db.UpsertSkill(sk); err != nil {
			t.Fatalf("UpsertSkill %s: %v", sk.Name, err)
		}
	}

	tests := []struct {
		name   string
		filter store.ListFilter
		want   int
	}{
		{"no filter", store.ListFilter{}, 3},
		{"by status", store.ListFilter{Status: "published"}, 2},
		{"by namespace", store.ListFilter{Namespace: "team2"}, 1},
		{"by query", store.ListFilter{Query: "kubernetes"}, 1},
		{"by tags", store.ListFilter{Tags: []string{"go"}}, 1},
		{"by multiple tags", store.ListFilter{Tags: []string{"go", "testing"}}, 1},
		{"by compatibility miss", store.ListFilter{Compatibility: "nonexistent"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.ListSkills(tt.filter)
			if err != nil {
				t.Fatalf("ListSkills: %v", err)
			}
			if len(result) != tt.want {
				t.Errorf("got %d skills, want %d", len(result), tt.want)
			}
		})
	}
}

func TestGetSkill(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.UpsertSkill(store.Skill{
		Repository: "team1/doc-reviewer", Tag: "1.0.0",
		Digest: "sha256:abc", Name: "doc-reviewer",
		Namespace: "team1", Status: "published",
	}); err != nil {
		t.Fatalf("UpsertSkill: %v", err)
	}

	sk, err := db.GetSkill("team1", "doc-reviewer")
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if sk.Name != "doc-reviewer" {
		t.Errorf("name = %q, want %q", sk.Name, "doc-reviewer")
	}

	_, err = db.GetSkill("team1", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestGetVersions(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, sk := range []store.Skill{
		{Repository: "team1/doc-reviewer", Tag: "1.0.0-draft", Digest: "sha256:aaa", Name: "doc-reviewer", Namespace: "team1", Version: "1.0.0", Status: "draft"},
		{Repository: "team1/doc-reviewer", Tag: "1.0.0", Digest: "sha256:bbb", Name: "doc-reviewer", Namespace: "team1", Version: "1.0.0", Status: "published"},
	} {
		if err := db.UpsertSkill(sk); err != nil {
			t.Fatalf("UpsertSkill: %v", err)
		}
	}

	versions, err := db.GetVersions("team1", "doc-reviewer")
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
}

func TestCountSkills(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, sk := range []store.Skill{
		{Repository: "team1/a", Tag: "1.0.0", Digest: "sha256:a", Name: "a", Namespace: "team1", Status: "published"},
		{Repository: "team1/b", Tag: "1.0.0", Digest: "sha256:b", Name: "b", Namespace: "team1", Status: "draft"},
		{Repository: "team2/c", Tag: "1.0.0", Digest: "sha256:c", Name: "c", Namespace: "team2", Status: "published"},
	} {
		if err := db.UpsertSkill(sk); err != nil {
			t.Fatalf("UpsertSkill: %v", err)
		}
	}

	count, err := db.CountSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("CountSkills: %v", err)
	}
	if count != 3 {
		t.Errorf("total count = %d, want 3", count)
	}

	count, err = db.CountSkills(store.ListFilter{Status: "published"})
	if err != nil {
		t.Fatalf("CountSkills: %v", err)
	}
	if count != 2 {
		t.Errorf("published count = %d, want 2", count)
	}
}

func TestDeleteStale(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.UpsertSkill(store.Skill{
		Repository: "team1/old", Tag: "1.0.0",
		Digest: "sha256:old", Name: "old", Namespace: "team1",
	}); err != nil {
		t.Fatalf("UpsertSkill: %v", err)
	}

	deleted, err := db.DeleteStale(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteStale: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	skills, err := db.ListSkills(store.ListFilter{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills after delete, got %d", len(skills))
	}
}

func TestPagination(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	for i := range 5 {
		if err := db.UpsertSkill(store.Skill{
			Repository: "team1/skill-" + string(rune('a'+i)),
			Tag:        "1.0.0",
			Digest:     "sha256:" + string(rune('a'+i)),
			Name:       "skill-" + string(rune('a'+i)),
			Namespace:  "team1",
		}); err != nil {
			t.Fatalf("UpsertSkill: %v", err)
		}
	}

	page1, err := db.ListSkills(store.ListFilter{Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf("ListSkills page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1: got %d skills, want 2", len(page1))
	}

	page3, err := db.ListSkills(store.ListFilter{Page: 3, PerPage: 2})
	if err != nil {
		t.Fatalf("ListSkills page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page 3: got %d skills, want 1", len(page3))
	}
}

func TestUpsertAndListCollections(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	col := store.Collection{
		Repository:  "quay.io/myorg/collections/hr-skills",
		Tag:         "1.0.0",
		Digest:      "sha256:abc123",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR skills collection",
		SkillsJSON:  `[{"name":"doc-summarizer","image":"quay.io/org/doc-summarizer:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}

	if err := db.UpsertCollection(col); err != nil {
		t.Fatalf("UpsertCollection: %v", err)
	}

	collections, err := db.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(collections))
	}
	if collections[0].Name != "hr-skills" {
		t.Errorf("name = %q, want %q", collections[0].Name, "hr-skills")
	}
}

func TestGetCollection(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	col := store.Collection{
		Repository:  "quay.io/myorg/collections/hr-skills",
		Tag:         "1.0.0",
		Digest:      "sha256:abc123",
		Name:        "hr-skills",
		Version:     "1.0.0",
		Description: "HR skills",
		SkillsJSON:  `[{"name":"s1","image":"quay.io/org/s1:1.0.0"}]`,
		Created:     "2026-04-27T10:00:00Z",
	}
	if err := db.UpsertCollection(col); err != nil {
		t.Fatalf("UpsertCollection: %v", err)
	}

	got, err := db.GetCollection("hr-skills")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", got.Version, "1.0.0")
	}
}

func TestGetCollectionNotFound(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.GetCollection("nonexistent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
