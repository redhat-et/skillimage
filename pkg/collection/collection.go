package collection

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type SkillCollection struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	Skills     []SkillRef `yaml:"skills"`
}

type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

type SkillRef struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

func Parse(r io.Reader) (*SkillCollection, error) {
	var col SkillCollection
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&col); err != nil {
		return nil, fmt.Errorf("parsing collection YAML: %w", err)
	}
	if col.Kind != "SkillCollection" {
		return nil, fmt.Errorf("expected kind SkillCollection, got %q", col.Kind)
	}
	return &col, nil
}

func ParseFile(path string) (*SkillCollection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening collection file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

func Validate(col *SkillCollection) []string {
	var errs []string
	if col.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if col.Metadata.Version == "" {
		errs = append(errs, "metadata.version is required")
	}
	if len(col.Skills) == 0 {
		errs = append(errs, "at least one skill is required")
		return errs
	}
	seen := make(map[string]bool)
	for i, s := range col.Skills {
		if s.Name == "" {
			errs = append(errs, fmt.Sprintf("skills[%d].name is required", i))
		}
		if s.Image == "" {
			errs = append(errs, fmt.Sprintf("skills[%d].image is required", i))
		}
		if s.Name != "" && seen[s.Name] {
			errs = append(errs, fmt.Sprintf("duplicate skill name %q", s.Name))
		}
		seen[s.Name] = true
	}
	return errs
}

// GenerateKubeYAML writes Kubernetes partial pod spec (volumes + volumeMounts) to w.
func GenerateKubeYAML(w io.Writer, col *SkillCollection, mountRoot string) {
	fmt.Fprintln(w, "volumes:")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "  - name: %s\n", s.Name)
		fmt.Fprintf(w, "    image:\n")
		fmt.Fprintf(w, "      reference: %s\n", s.Image)
		fmt.Fprintf(w, "      pullPolicy: IfNotPresent\n")
	}
	fmt.Fprintln(w, "containers:")
	fmt.Fprintln(w, "  - name: agent")
	fmt.Fprintln(w, "    volumeMounts:")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "      - name: %s\n", s.Name)
		fmt.Fprintf(w, "        mountPath: %s/%s\n", mountRoot, s.Name)
		fmt.Fprintf(w, "        readOnly: true\n")
	}
}

// GeneratePodmanVolumes writes Podman volume creation commands to w.
func GeneratePodmanVolumes(w io.Writer, col *SkillCollection, mountRoot string) {
	for _, s := range col.Skills {
		fmt.Fprintf(w, "podman pull %s\n", s.Image)
		fmt.Fprintf(w, "podman volume create --driver image \\\n  --opt image=%s \\\n  %s\n\n", s.Image, s.Name)
	}
	fmt.Fprintf(w, "# Run with:\n# podman run --rm \\\n")
	for _, s := range col.Skills {
		fmt.Fprintf(w, "#   -v %s:%s/%s:ro \\\n", s.Name, mountRoot, s.Name)
	}
	fmt.Fprintf(w, "#   my-agent:latest\n")
}
