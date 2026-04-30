package installed_test

import (
	"context"
	"testing"

	"github.com/redhat-et/skillimage/pkg/installed"
)

func TestCheckUpgrades_HasUpgrade(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "my-skill",
			Version: "1.0.0",
			Source:  "quay.io/acme/my-skill:1.0.0",
			Target:  "claude",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		return []string{"1.0.0-draft", "1.0.0-testing", "1.0.0", "2.0.0-draft", "2.0.0"}, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].LatestVersion != "2.0.0" {
		t.Errorf("latest = %q, want %q", candidates[0].LatestVersion, "2.0.0")
	}
	if candidates[0].LatestRef != "quay.io/acme/my-skill:2.0.0" {
		t.Errorf("ref = %q, want %q", candidates[0].LatestRef, "quay.io/acme/my-skill:2.0.0")
	}
}

func TestCheckUpgrades_AlreadyLatest(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "my-skill",
			Version: "2.0.0",
			Source:  "quay.io/acme/my-skill:2.0.0",
			Target:  "claude",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		return []string{"1.0.0", "2.0.0"}, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestCheckUpgrades_NoProvenance(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "local-skill",
			Version: "1.0.0",
			Source:  "",
			Target:  "claude",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		t.Fatal("should not be called for local skills")
		return nil, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestCheckUpgrades_OnlyDraftTags(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "my-skill",
			Version: "1.0.0",
			Source:  "quay.io/acme/my-skill:1.0.0",
			Target:  "claude",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		return []string{"1.0.0", "2.0.0-draft", "3.0.0-testing"}, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates (only draft/testing newer), got %d", len(candidates))
	}
}

func TestCheckUpgrades_LocalRef(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "my-skill",
			Version: "1.0.0",
			Source:  "toddward/red-hat-quick-deck:0.1.0-draft",
			Target:  "opencode",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		t.Fatal("should not be called for local refs")
		return nil, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for local ref, got %d", len(candidates))
	}
}

func TestCheckUpgrades_InvalidSemver(t *testing.T) {
	skills := []installed.InstalledSkill{
		{
			Name:    "my-skill",
			Version: "not-a-version",
			Source:  "quay.io/acme/my-skill:latest",
			Target:  "claude",
		},
	}

	lister := func(ctx context.Context, repo string, skipTLS bool) ([]string, error) {
		t.Fatal("should not be called for non-semver versions")
		return nil, nil
	}

	candidates, err := installed.CheckUpgrades(context.Background(), skills,
		installed.CheckOptions{TagLister: lister})
	if err != nil {
		t.Fatalf("CheckUpgrades: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}
