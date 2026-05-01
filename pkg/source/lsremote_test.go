package source

import (
	"context"
	"testing"
)

func TestLsRemoteReturnsCommitSHA(t *testing.T) {
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	// Use a well-known public repo with a known branch.
	sha, err := LsRemote(context.Background(), "https://github.com/octocat/Hello-World.git", "master")
	if err != nil {
		t.Fatalf("LsRemote: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("expected commit SHA, got %q", sha)
	}
	if !commitSHAPattern.MatchString(sha) {
		t.Errorf("SHA %q does not match commit pattern", sha)
	}
}

func TestLsRemoteBadRef(t *testing.T) {
	if err := CheckGit(); err != nil {
		t.Skip("git not available")
	}

	_, err := LsRemote(context.Background(), "https://github.com/octocat/Hello-World.git", "nonexistent-branch-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}
