package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// Discoverer lists OCI repository names from a registry.
type Discoverer interface {
	Discover(ctx context.Context) ([]string, error)
}

// RegistryType selects which discovery adapter to use.
type RegistryType string

const (
	RegistryTypeAuto RegistryType = "auto"
	RegistryTypeOCI  RegistryType = "oci"
	RegistryTypeQuay RegistryType = "quay"
)

// DiscoveryConfig holds settings for constructing a Discoverer.
type DiscoveryConfig struct {
	RegistryURL   string
	Namespace     string
	SkipTLSVerify bool
	RegistryType  RegistryType
}

// NewDiscoverer creates a Discoverer based on the registry type.
// If RegistryType is "auto" or empty, it inspects the registry URL
// to select the best adapter.
func NewDiscoverer(cfg DiscoveryConfig) Discoverer {
	rt := cfg.RegistryType
	if rt == "" || rt == RegistryTypeAuto {
		rt = DetectRegistryType(cfg.RegistryURL)
	}
	switch rt {
	case RegistryTypeQuay:
		return &QuayDiscoverer{
			registryURL:   cfg.RegistryURL,
			namespace:     cfg.Namespace,
			skipTLSVerify: cfg.SkipTLSVerify,
		}
	default:
		return &OCICatalogDiscoverer{
			registryURL:   cfg.RegistryURL,
			prefix:        cfg.Namespace,
			skipTLSVerify: cfg.SkipTLSVerify,
		}
	}
}

// DetectRegistryType inspects the registry URL to determine
// which discovery adapter to use.
func DetectRegistryType(registryURL string) RegistryType {
	if strings.Contains(strings.ToLower(registryURL), "quay") {
		return RegistryTypeQuay
	}
	return RegistryTypeOCI
}

// OCICatalogDiscoverer uses the standard OCI /v2/_catalog endpoint.
// Works with Harbor, Zot, distribution, and OpenShift internal registries.
type OCICatalogDiscoverer struct {
	registryURL   string
	prefix        string
	skipTLSVerify bool
}

func (d *OCICatalogDiscoverer) Discover(ctx context.Context) ([]string, error) {
	return ListRemoteRepositories(ctx, d.registryURL, d.prefix, d.skipTLSVerify)
}

// QuayDiscoverer uses the Quay REST API to list repositories in an organization.
// Works with quay.io and self-hosted Quay instances.
type QuayDiscoverer struct {
	registryURL   string
	namespace     string
	skipTLSVerify bool
	// apiBaseURL overrides the API base URL (for testing with httptest).
	apiBaseURL string
	// httpClient overrides the default HTTP client (for testing).
	httpClient *http.Client
}

func (d *QuayDiscoverer) Discover(ctx context.Context) ([]string, error) {
	if d.namespace == "" {
		return nil, fmt.Errorf("quay discovery requires --namespace (organization name)")
	}

	base := d.apiBaseURL
	if base == "" {
		host := d.registryURL
		if strings.HasPrefix(host, "https://") || strings.HasPrefix(host, "http://") {
			base = host
		} else {
			base = "https://" + host
		}
	}

	token := d.loadAuthToken()

	q := url.Values{}
	q.Set("namespace", d.namespace)
	q.Set("public", "true")
	apiURL := base + "/api/v1/repository?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating quay API request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		slog.Debug("using credentials for quay API")
	}

	client := d.httpClient
	if client == nil {
		if d.skipTLSVerify {
			client = insecureHTTPClient()
		} else {
			client = http.DefaultClient
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("quay API request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quay API returned status %d", resp.StatusCode)
	}

	var result struct {
		Repositories []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding quay API response: %w", err)
	}

	repos := make([]string, 0, len(result.Repositories))
	for _, r := range result.Repositories {
		repos = append(repos, r.Namespace+"/"+r.Name)
	}
	return repos, nil
}

// NewQuayDiscovererForTest creates a QuayDiscoverer pointing at a test server.
func NewQuayDiscovererForTest(baseURL, namespace string, client *http.Client) *QuayDiscoverer {
	return &QuayDiscoverer{
		registryURL: baseURL,
		namespace:   namespace,
		apiBaseURL:  baseURL,
		httpClient:  client,
	}
}

func (d *QuayDiscoverer) loadAuthToken() string {
	store, err := credentialStore()
	if err != nil {
		return ""
	}
	cred, err := store.Get(context.Background(), d.registryURL)
	if err != nil {
		return ""
	}
	if cred.AccessToken != "" {
		return cred.AccessToken
	}
	if cred.Password != "" {
		return cred.Password
	}
	return ""
}
