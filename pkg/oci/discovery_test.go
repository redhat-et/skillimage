package oci_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhat-et/skillimage/pkg/oci"
)

func TestQuayDiscoverer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository" {
			t.Errorf("path = %q, want /api/v1/repository", r.URL.Path)
		}
		if ns := r.URL.Query().Get("namespace"); ns != "myorg" {
			t.Errorf("namespace = %q, want myorg", ns)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"repositories": []map[string]string{
				{"namespace": "myorg", "name": "skill-a"},
				{"namespace": "myorg", "name": "skill-b"},
				{"namespace": "myorg", "name": "nested/skill-c"},
			},
		})
	}))
	defer srv.Close()

	d := oci.NewQuayDiscovererForTest(srv.URL, "myorg", srv.Client())
	repos, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := []string{"myorg/skill-a", "myorg/skill-b", "myorg/nested/skill-c"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos, want %d", len(repos), len(want))
	}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("repos[%d] = %q, want %q", i, r, want[i])
		}
	}
}

func TestQuayDiscovererEmptyNamespace(t *testing.T) {
	d := oci.NewQuayDiscovererForTest("http://localhost", "", nil)
	_, err := d.Discover(context.Background())
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestQuayDiscovererAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := oci.NewQuayDiscovererForTest(srv.URL, "myorg", srv.Client())
	_, err := d.Discover(context.Background())
	if err == nil {
		t.Fatal("expected error for API 500")
	}
}

func TestQuayDiscovererEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"repositories": []map[string]string{},
		})
	}))
	defer srv.Close()

	d := oci.NewQuayDiscovererForTest(srv.URL, "myorg", srv.Client())
	repos, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("got %d repos, want 0", len(repos))
	}
}

func TestDetectRegistryType(t *testing.T) {
	tests := []struct {
		url  string
		want oci.RegistryType
	}{
		{"quay.io", oci.RegistryTypeQuay},
		{"Quay.io", oci.RegistryTypeQuay},
		{"quay.example.com", oci.RegistryTypeQuay},
		{"registry.quay.internal", oci.RegistryTypeQuay},
		{"registry.example.com", oci.RegistryTypeOCI},
		{"harbor.internal", oci.RegistryTypeOCI},
		{"localhost:5000", oci.RegistryTypeOCI},
		{"image-registry.openshift-image-registry.svc:5000", oci.RegistryTypeOCI},
		{"ghcr.io", oci.RegistryTypeOCI},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := oci.DetectRegistryType(tt.url)
			if got != tt.want {
				t.Errorf("DetectRegistryType(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestNewDiscovererAutoDetectsQuay(t *testing.T) {
	d := oci.NewDiscoverer(oci.DiscoveryConfig{
		RegistryURL:  "quay.io",
		Namespace:    "myorg",
		RegistryType: oci.RegistryTypeAuto,
	})
	if _, ok := d.(*oci.QuayDiscoverer); !ok {
		t.Errorf("expected *QuayDiscoverer for quay.io, got %T", d)
	}
}

func TestNewDiscovererDefaultsToOCI(t *testing.T) {
	d := oci.NewDiscoverer(oci.DiscoveryConfig{
		RegistryURL:  "registry.example.com",
		RegistryType: oci.RegistryTypeAuto,
	})
	if _, ok := d.(*oci.OCICatalogDiscoverer); !ok {
		t.Errorf("expected *OCICatalogDiscoverer for registry.example.com, got %T", d)
	}
}

func TestNewDiscovererExplicitOverride(t *testing.T) {
	d := oci.NewDiscoverer(oci.DiscoveryConfig{
		RegistryURL:  "registry.example.com",
		Namespace:    "myorg",
		RegistryType: oci.RegistryTypeQuay,
	})
	if _, ok := d.(*oci.QuayDiscoverer); !ok {
		t.Errorf("expected *QuayDiscoverer with explicit override, got %T", d)
	}
}
