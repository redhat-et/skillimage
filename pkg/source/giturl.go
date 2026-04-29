package source

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

type GitSource struct {
	CloneURL string
	Ref      string
	SubPath  string
}

func ParseGitURL(raw string) (GitSource, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return GitSource{}, fmt.Errorf("not a valid URL: %s", raw)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return GitSource{}, fmt.Errorf("URL must contain at least org/repo: %s", raw)
	}

	host := u.Host

	switch {
	case host == "github.com":
		return parseGitHub(u, parts)
	case host == "gitlab.com" || containsGitLab(parts):
		return parseGitLab(u, parts)
	default:
		return parseGeneric(u, parts)
	}
}

func parseGitHub(u *url.URL, parts []string) (GitSource, error) {
	org := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", u.Host, org, repo)

	var ref, subPath string
	if len(parts) > 3 && parts[2] == "tree" {
		ref = parts[3]
		if len(parts) > 4 {
			subPath = strings.Join(parts[4:], "/")
		}
	}

	return GitSource{CloneURL: cloneURL, Ref: ref, SubPath: subPath}, nil
}

func parseGitLab(u *url.URL, parts []string) (GitSource, error) {
	org := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", u.Host, org, repo)

	var ref, subPath string
	for i, p := range parts {
		if p == "-" && i+2 < len(parts) && parts[i+1] == "tree" {
			ref = parts[i+2]
			if i+3 < len(parts) {
				subPath = strings.Join(parts[i+3:], "/")
			}
			break
		}
	}

	return GitSource{CloneURL: cloneURL, Ref: ref, SubPath: subPath}, nil
}

func parseGeneric(u *url.URL, _ []string) (GitSource, error) {
	return GitSource{
		CloneURL: u.String(),
		Ref:      "",
		SubPath:  "",
	}, nil
}

func containsGitLab(parts []string) bool {
	return slices.Contains(parts, "-")
}
