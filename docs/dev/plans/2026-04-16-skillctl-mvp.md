# skillctl MVP implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a working `skillctl` CLI that packs, pushes,
pulls, inspects, and promotes AI agent skills as OCI images with
lifecycle governance.

**Architecture:** Library-first Go project. Core logic in `pkg/`
(skillcard, oci, lifecycle), CLI in `internal/cli/` + `cmd/skillctl/`.
Skills are OCI images built from scratch with lifecycle status
tracked via OCI manifest annotations. A `Client` struct in `pkg/oci/`
manages a local OCI layout store and remote registry operations.

**Tech Stack:** Go 1.26, oras-go v2.6.0, Cobra, Viper,
santhosh-tekuri/jsonschema v6, Masterminds/semver v3, gopkg.in/yaml.v3

---

## File structure

| File | Responsibility |
| ---- | -------------- |
| `schemas/skillcard-v1.json` | JSON Schema for SkillCard validation |
| `schemas/embed.go` | Embeds JSON Schema for use in Go |
| `pkg/skillcard/skillcard.go` | SkillCard types, Parse, Serialize |
| `pkg/skillcard/validate.go` | Validate against JSON Schema + semver |
| `pkg/skillcard/skillcard_test.go` | Tests for Parse, Serialize, Validate |
| `pkg/lifecycle/lifecycle.go` | State type, transitions, tag rules |
| `pkg/lifecycle/lifecycle_test.go` | Tests for state machine |
| `pkg/oci/client.go` | Client struct, store management |
| `pkg/oci/annotations.go` | OCI annotation mapping from SkillCard |
| `pkg/oci/pack.go` | Pack skill directory into OCI image |
| `pkg/oci/push.go` | Push from local store to remote registry |
| `pkg/oci/pull.go` | Pull from remote to local store, unpack |
| `pkg/oci/inspect.go` | Inspect local or remote skill images |
| `pkg/oci/promote.go` | Promote lifecycle state on remote registry |
| `pkg/oci/oci_test.go` | Tests for all OCI operations |
| `internal/cli/root.go` | Root Cobra command, global flags |
| `internal/cli/validate.go` | validate subcommand |
| `internal/cli/pack.go` | pack subcommand |
| `internal/cli/images.go` | images subcommand |
| `internal/cli/push.go` | push subcommand |
| `internal/cli/pull.go` | pull subcommand |
| `internal/cli/inspect.go` | inspect subcommand |
| `internal/cli/promote.go` | promote subcommand |
| `cmd/skillctl/main.go` | Entry point |
| `examples/hello-world/skill.yaml` | Sample SkillCard |
| `examples/hello-world/SKILL.md` | Sample skill prompt |

---

## Task 1: Project scaffolding

**Files:**

- Modify: `go.mod`
- Create: `schemas/skillcard-v1.json`
- Create: `schemas/embed.go`
- Create: `examples/hello-world/skill.yaml`
- Create: `examples/hello-world/SKILL.md`

- [ ] **Step 1: Add dependencies**

```bash
cd /Users/panni/work/oci-skill-registry
go get oras.land/oras-go/v2@latest
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get github.com/santhosh-tekuri/jsonschema/v6@latest
go get github.com/Masterminds/semver/v3@latest
go get gopkg.in/yaml.v3@latest
go get github.com/opencontainers/image-spec@latest
```

Run: `go mod tidy`

- [ ] **Step 2: Create JSON Schema**

Create `schemas/skillcard-v1.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://skills.redhat.io/schemas/skillcard-v1.json",
  "title": "SkillCard",
  "description": "Schema for AI agent skill metadata",
  "type": "object",
  "required": ["apiVersion", "kind", "metadata"],
  "additionalProperties": false,
  "properties": {
    "apiVersion": {
      "type": "string",
      "const": "skillimage.io/v1alpha1"
    },
    "kind": {
      "type": "string",
      "const": "SkillCard"
    },
    "metadata": {
      "type": "object",
      "required": ["name", "namespace", "version", "description"],
      "additionalProperties": false,
      "properties": {
        "name": {
          "type": "string",
          "minLength": 1,
          "maxLength": 64,
          "pattern": "^[a-z0-9]+(-[a-z0-9]+)*$"
        },
        "namespace": {
          "type": "string",
          "minLength": 1,
          "maxLength": 128,
          "pattern": "^[a-z0-9]+(-[a-z0-9]+)*(/[a-z0-9]+(-[a-z0-9]+)*)*$"
        },
        "version": {
          "type": "string",
          "minLength": 1
        },
        "description": {
          "type": "string",
          "minLength": 1
        },
        "display-name": {
          "type": "string",
          "minLength": 1,
          "maxLength": 128
        },
        "license": {
          "type": "string"
        },
        "compatibility": {
          "type": "string"
        },
        "tags": {
          "type": "array",
          "items": { "type": "string" }
        },
        "authors": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name"],
            "additionalProperties": false,
            "properties": {
              "name": { "type": "string", "minLength": 1 },
              "email": { "type": "string" }
            }
          }
        },
        "allowed-tools": {
          "type": "string"
        }
      }
    },
    "provenance": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "source": { "type": "string" },
        "commit": { "type": "string" },
        "path": { "type": "string" }
      }
    },
    "spec": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "prompt": { "type": "string" },
        "examples": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
              "input": { "type": "string" },
              "output": { "type": "string" }
            }
          }
        },
        "dependencies": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name", "version"],
            "additionalProperties": false,
            "properties": {
              "name": { "type": "string" },
              "version": { "type": "string" }
            }
          }
        }
      }
    }
  }
}
```

- [ ] **Step 3: Create schema embed package**

Create `schemas/embed.go`:

```go
package schemas

import _ "embed"

//go:embed skillcard-v1.json
var SkillCardV1 []byte
```

- [ ] **Step 4: Create example skill**

Create `examples/hello-world/skill.yaml`:

```yaml
apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: hello-world
  namespace: examples
  version: 1.0.0
  display-name: "Hello World"
  description: >
    A simple example skill that greets the user.
    Use this as a template for creating new skills.
  license: Apache-2.0
  tags:
    - example
    - getting-started
  authors:
    - name: OCTO Team
      email: octo@redhat.com
spec:
  prompt: SKILL.md
  examples:
    - input: "Hello"
      output: "Hello! How can I help you today?"
```

Create `examples/hello-world/SKILL.md`:

```markdown
You are a friendly greeter skill.

When the user says hello, greet them warmly and ask how you
can help. Keep responses concise and helpful.
```

- [ ] **Step 5: Verify build**

Run: `go build ./schemas/`
Expected: clean build, no errors

- [ ] **Step 6: Commit**

```bash
git add schemas/ examples/ go.mod go.sum
git commit -s -m "feat: add JSON Schema, embed package, and example skill"
```

---

## Task 2: SkillCard types, Parse, and Serialize

**Files:**

- Create: `pkg/skillcard/skillcard.go`
- Create: `pkg/skillcard/skillcard_test.go`

- [ ] **Step 1: Write tests for Parse and Serialize**

Create `pkg/skillcard/skillcard_test.go`:

```go
package skillcard_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
)

const validSkillYAML = `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: hello-world
  namespace: examples
  version: 1.0.0
  display-name: "Hello World"
  description: A simple example skill.
  license: Apache-2.0
  tags:
    - example
  authors:
    - name: Test Author
      email: test@example.com
spec:
  prompt: SKILL.md
`

func TestParse(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.APIVersion != "skillimage.io/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", sc.APIVersion, "skillimage.io/v1alpha1")
	}
	if sc.Kind != "SkillCard" {
		t.Errorf("kind = %q, want %q", sc.Kind, "SkillCard")
	}
	if sc.Metadata.Name != "hello-world" {
		t.Errorf("name = %q, want %q", sc.Metadata.Name, "hello-world")
	}
	if sc.Metadata.Namespace != "examples" {
		t.Errorf("namespace = %q, want %q", sc.Metadata.Namespace, "examples")
	}
	if sc.Metadata.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", sc.Metadata.Version, "1.0.0")
	}
	if sc.Metadata.DisplayName != "Hello World" {
		t.Errorf("display-name = %q, want %q", sc.Metadata.DisplayName, "Hello World")
	}
	if len(sc.Metadata.Authors) != 1 || sc.Metadata.Authors[0].Name != "Test Author" {
		t.Errorf("authors = %v, want [{Test Author test@example.com}]", sc.Metadata.Authors)
	}
	if sc.Spec == nil || sc.Spec.Prompt != "SKILL.md" {
		t.Errorf("spec.prompt = %v, want SKILL.md", sc.Spec)
	}
}

func TestParseInvalidYAML(t *testing.T) {
	_, err := skillcard.Parse(strings.NewReader("not: [valid: yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSerialize(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	if err := skillcard.Serialize(sc, &buf); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	roundtrip, err := skillcard.Parse(&buf)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if roundtrip.Metadata.Name != sc.Metadata.Name {
		t.Errorf("roundtrip name = %q, want %q", roundtrip.Metadata.Name, sc.Metadata.Name)
	}
	if roundtrip.Metadata.Version != sc.Metadata.Version {
		t.Errorf("roundtrip version = %q, want %q", roundtrip.Metadata.Version, sc.Metadata.Version)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/skillcard/ -v`
Expected: compilation error — `skillcard` package does not exist

- [ ] **Step 3: Implement SkillCard types, Parse, and Serialize**

Create `pkg/skillcard/skillcard.go`:

```go
package skillcard

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

type SkillCard struct {
	APIVersion string      `yaml:"apiVersion" json:"api_version"`
	Kind       string      `yaml:"kind" json:"kind"`
	Metadata   Metadata    `yaml:"metadata" json:"metadata"`
	Provenance *Provenance `yaml:"provenance,omitempty" json:"provenance,omitempty"`
	Spec       *Spec       `yaml:"spec,omitempty" json:"spec,omitempty"`
}

type Metadata struct {
	Name          string   `yaml:"name" json:"name"`
	DisplayName   string   `yaml:"display-name,omitempty" json:"display_name,omitempty"`
	Namespace     string   `yaml:"namespace" json:"namespace"`
	Version       string   `yaml:"version" json:"version"`
	Description   string   `yaml:"description" json:"description"`
	License       string   `yaml:"license,omitempty" json:"license,omitempty"`
	Compatibility string   `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Tags          []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Authors       []Author `yaml:"authors,omitempty" json:"authors,omitempty"`
	AllowedTools  string   `yaml:"allowed-tools,omitempty" json:"allowed_tools,omitempty"`
}

type Author struct {
	Name  string `yaml:"name" json:"name"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
}

type Provenance struct {
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	Commit string `yaml:"commit,omitempty" json:"commit,omitempty"`
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
}

type Spec struct {
	Prompt       string       `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Examples     []Example    `yaml:"examples,omitempty" json:"examples,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
}

type Example struct {
	Input  string `yaml:"input" json:"input"`
	Output string `yaml:"output" json:"output"`
}

type Dependency struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

func Parse(r io.Reader) (*SkillCard, error) {
	var sc SkillCard
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&sc); err != nil {
		return nil, fmt.Errorf("parsing skillcard YAML: %w", err)
	}
	return &sc, nil
}

func Serialize(sc *SkillCard, w io.Writer) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(sc); err != nil {
		return fmt.Errorf("serializing skillcard: %w", err)
	}
	return enc.Close()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/skillcard/ -v`
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/skillcard/
git commit -s -m "feat: add SkillCard types with Parse and Serialize"
```

---

## Task 3: SkillCard validation

**Files:**

- Create: `pkg/skillcard/validate.go`
- Modify: `pkg/skillcard/skillcard_test.go` (add validation tests)

- [ ] **Step 1: Write validation tests**

Append to `pkg/skillcard/skillcard_test.go`:

```go
func TestValidateValid(t *testing.T) {
	sc, err := skillcard.Parse(strings.NewReader(validSkillYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, err := skillcard.Validate(sc)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %v", errs)
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	yaml := `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, err := skillcard.Validate(sc)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing required fields")
	}
	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, required := range []string{"namespace", "version", "description"} {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Field, required) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for missing %q, got errors: %v", required, errs)
		}
	}
}

func TestValidateInvalidName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid", "hello-world", false},
		{"valid single char", "a", false},
		{"uppercase", "Hello", true},
		{"spaces", "hello world", true},
		{"leading hyphen", "-hello", true},
		{"trailing hyphen", "hello-", true},
		{"consecutive hyphens", "hello--world", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: %s
  namespace: test
  version: 1.0.0
  description: test
`, tt.value)
			sc, err := skillcard.Parse(strings.NewReader(yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			errs, _ := skillcard.Validate(sc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("name=%q: hasErr=%v, wantErr=%v, errs=%v",
					tt.value, hasErr, tt.wantErr, errs)
			}
		})
	}
}

func TestValidateInvalidSemver(t *testing.T) {
	yaml := `apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test
  namespace: test
  version: not-semver
  description: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, _ := skillcard.Validate(sc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Field, "version") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected semver validation error, got: %v", errs)
	}
}

func TestValidateWrongAPIVersion(t *testing.T) {
	yaml := `apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: test
  namespace: test
  version: 1.0.0
  description: test
`
	sc, err := skillcard.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs, _ := skillcard.Validate(sc)
	if len(errs) == 0 {
		t.Fatal("expected error for wrong apiVersion")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/skillcard/ -run TestValidate -v`
Expected: compilation error — `Validate` and `ValidationError`
not defined

- [ ] **Step 3: Implement Validate**

Create `pkg/skillcard/validate.go`:

```go
package skillcard

import (
	"encoding/json"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/redhat-et/oci-skill-registry/schemas"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) String() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

var compiledSchema *jsonschema.Schema

func init() {
	var schemaDoc any
	if err := json.Unmarshal(schemas.SkillCardV1, &schemaDoc); err != nil {
		panic(fmt.Sprintf("unmarshaling embedded schema: %v", err))
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("skillcard-v1.json", schemaDoc); err != nil {
		panic(fmt.Sprintf("adding schema resource: %v", err))
	}
	var err error
	compiledSchema, err = compiler.Compile("skillcard-v1.json")
	if err != nil {
		panic(fmt.Sprintf("compiling schema: %v", err))
	}
}

func Validate(sc *SkillCard) ([]ValidationError, error) {
	yamlBytes, err := yaml.Marshal(sc)
	if err != nil {
		return nil, fmt.Errorf("marshaling for validation: %w", err)
	}
	var raw any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, fmt.Errorf("unmarshaling for validation: %w", err)
	}

	var errs []ValidationError

	if err := compiledSchema.Validate(raw); err != nil {
		vErr, ok := err.(*jsonschema.ValidationError)
		if ok {
			errs = append(errs, collectSchemaErrors(vErr)...)
		} else {
			return nil, fmt.Errorf("schema validation: %w", err)
		}
	}

	if sc.Metadata.Version != "" {
		if _, err := semver.StrictNewVersion(sc.Metadata.Version); err != nil {
			errs = append(errs, ValidationError{
				Field:   "metadata.version",
				Message: fmt.Sprintf("must be valid semver: %v", err),
			})
		}
	}

	return errs, nil
}

func collectSchemaErrors(vErr *jsonschema.ValidationError) []ValidationError {
	if len(vErr.Causes) == 0 {
		field := vErr.InstanceLocation
		if field == "" {
			field = "/"
		}
		return []ValidationError{{
			Field:   field,
			Message: vErr.Error(),
		}}
	}
	var errs []ValidationError
	for _, cause := range vErr.Causes {
		errs = append(errs, collectSchemaErrors(cause)...)
	}
	return errs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/skillcard/ -v`
Expected: all tests PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./pkg/skillcard/`
Expected: no issues

- [ ] **Step 6: Commit**

```bash
git add pkg/skillcard/validate.go pkg/skillcard/skillcard_test.go
git commit -s -m "feat: add SkillCard validation with JSON Schema and semver"
```

---

## Task 4: CLI framework and skillctl validate

**Files:**

- Create: `internal/cli/root.go`
- Create: `internal/cli/validate.go`
- Create: `cmd/skillctl/main.go`

- [ ] **Step 1: Create root command**

Create `internal/cli/root.go`:

```go
package cli

import (
	"github.com/spf13/cobra"
)

var (
	flagFormat  string
	flagVerbose bool
)

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skillctl",
		Short: "Manage AI agent skills as OCI images",
		Long:  "skillctl packs, pushes, pulls, and manages the lifecycle of AI agent skills stored as OCI images.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&flagFormat, "format", "text", "output format (text, json)")
	cmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")

	cmd.AddCommand(newValidateCmd())

	return cmd
}
```

- [ ] **Step 2: Create validate command**

Create `internal/cli/validate.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <dir|file>",
		Short: "Validate a SkillCard against the JSON Schema",
		Args:  cobra.ExactArgs(1),
		RunE:  runValidate,
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("accessing %s: %w", path, err)
	}
	if info.IsDir() {
		path = filepath.Join(path, "skill.yaml")
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	errs, err := skillcard.Validate(sc)
	if err != nil {
		return fmt.Errorf("validating %s: %w", path, err)
	}

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "✗ %s has %d error(s):\n", path, len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", e.Field, e.Message)
		}
		os.Exit(1)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ %s is valid\n", path)
	return nil
}
```

- [ ] **Step 3: Create main.go**

Create `cmd/skillctl/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/redhat-et/oci-skill-registry/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCmd(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
```

- [ ] **Step 4: Build and test with example skill**

Run: `go build -o bin/skillctl ./cmd/skillctl`
Expected: clean build

Run: `./bin/skillctl validate examples/hello-world/`
Expected: `✓ examples/hello-world/skill.yaml is valid`

- [ ] **Step 5: Test with invalid input**

Create a temporary invalid skill:

```bash
mkdir -p /tmp/bad-skill
cat > /tmp/bad-skill/skill.yaml << 'EOF'
apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: BAD NAME
  namespace: test
  version: not-semver
  description: test
EOF
```

Run: `./bin/skillctl validate /tmp/bad-skill/`
Expected: exit code 1 with validation errors for apiVersion,
name pattern, and semver

- [ ] **Step 6: Commit**

```bash
git add cmd/skillctl/ internal/cli/
git commit -s -m "feat: add CLI framework and skillctl validate command"
```

---

## Task 5: Lifecycle state machine

**Files:**

- Create: `pkg/lifecycle/lifecycle.go`
- Create: `pkg/lifecycle/lifecycle_test.go`

- [ ] **Step 1: Write lifecycle tests**

Create `pkg/lifecycle/lifecycle_test.go`:

```go
package lifecycle_test

import (
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from lifecycle.State
		to   lifecycle.State
		want bool
	}{
		{lifecycle.Draft, lifecycle.Testing, true},
		{lifecycle.Testing, lifecycle.Published, true},
		{lifecycle.Published, lifecycle.Deprecated, true},
		{lifecycle.Deprecated, lifecycle.Archived, true},
		// Invalid transitions
		{lifecycle.Draft, lifecycle.Published, false},
		{lifecycle.Draft, lifecycle.Deprecated, false},
		{lifecycle.Testing, lifecycle.Draft, false},
		{lifecycle.Published, lifecycle.Testing, false},
		{lifecycle.Published, lifecycle.Archived, false},
		{lifecycle.Archived, lifecycle.Draft, false},
		{lifecycle.Archived, lifecycle.Published, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := lifecycle.ValidTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("ValidTransition(%s, %s) = %v, want %v",
					tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestTagForState(t *testing.T) {
	tests := []struct {
		version string
		state   lifecycle.State
		want    string
	}{
		{"1.2.0", lifecycle.Draft, "1.2.0-draft"},
		{"1.2.0", lifecycle.Testing, "1.2.0-rc"},
		{"1.2.0", lifecycle.Published, "1.2.0"},
		{"1.2.0", lifecycle.Deprecated, "1.2.0"},
		{"1.2.0", lifecycle.Archived, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := lifecycle.TagForState(tt.version, tt.state)
			if got != tt.want {
				t.Errorf("TagForState(%q, %s) = %q, want %q",
					tt.version, tt.state, got, tt.want)
			}
		})
	}
}

func TestParseState(t *testing.T) {
	tests := []struct {
		input   string
		want    lifecycle.State
		wantErr bool
	}{
		{"draft", lifecycle.Draft, false},
		{"testing", lifecycle.Testing, false},
		{"published", lifecycle.Published, false},
		{"deprecated", lifecycle.Deprecated, false},
		{"archived", lifecycle.Archived, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := lifecycle.ParseState(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseState(%q) error = %v, wantErr %v",
					tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseState(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/lifecycle/ -v`
Expected: compilation error — package does not exist

- [ ] **Step 3: Implement lifecycle**

Create `pkg/lifecycle/lifecycle.go`:

```go
package lifecycle

import (
	"errors"
	"fmt"
)

type State string

const (
	Draft      State = "draft"
	Testing    State = "testing"
	Published  State = "published"
	Deprecated State = "deprecated"
	Archived   State = "archived"
)

const StatusAnnotation = "io.skillimage.status"

var transitions = map[State]State{
	Draft:      Testing,
	Testing:    Published,
	Published:  Deprecated,
	Deprecated: Archived,
}

var errInvalidState = errors.New("invalid lifecycle state")

func ValidTransition(from, to State) bool {
	next, ok := transitions[from]
	return ok && next == to
}

func TagForState(version string, state State) string {
	switch state {
	case Draft:
		return version + "-draft"
	case Testing:
		return version + "-rc"
	case Published:
		return version
	case Deprecated:
		return version
	case Archived:
		return ""
	default:
		return ""
	}
}

func ParseState(s string) (State, error) {
	switch State(s) {
	case Draft, Testing, Published, Deprecated, Archived:
		return State(s), nil
	default:
		return "", fmt.Errorf("%w: %q", errInvalidState, s)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/lifecycle/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/lifecycle/
git commit -s -m "feat: add lifecycle state machine with transitions and tag rules"
```

---

## Task 6: OCI Client, Pack, and ListLocal

**Files:**

- Create: `pkg/oci/client.go`
- Create: `pkg/oci/annotations.go`
- Create: `pkg/oci/pack.go`
- Create: `pkg/oci/oci_test.go`

- [ ] **Step 1: Write tests for Pack and ListLocal**

Create `pkg/oci/oci_test.go`:

```go
package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
)

func writeTestSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillYAML := []byte(`apiVersion: skillimage.io/v1alpha1
kind: SkillCard
metadata:
  name: test-skill
  namespace: test
  version: 1.0.0
  description: A test skill.
spec:
  prompt: SKILL.md
`)
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), skillYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Test prompt content."), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPackAndListLocal(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	desc, err := client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}

	images, err := client.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].Name != "test/test-skill" {
		t.Errorf("image name = %q, want %q", images[0].Name, "test/test-skill")
	}
	if images[0].Version != "1.0.0" {
		t.Errorf("image version = %q, want %q", images[0].Version, "1.0.0")
	}
	if images[0].Tag != "1.0.0-draft" {
		t.Errorf("image tag = %q, want %q", images[0].Tag, "1.0.0-draft")
	}
}

func TestPackValidatesSkillCard(t *testing.T) {
	skillDir := t.TempDir()
	badYAML := []byte(`apiVersion: wrong/v1
kind: SkillCard
metadata:
  name: BAD
  namespace: test
  version: 1.0.0
  description: test
`)
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), badYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Pack(context.Background(), skillDir, oci.PackOptions{})
	if err == nil {
		t.Fatal("expected error for invalid SkillCard")
	}
}

func TestPackMissingSkillYAML(t *testing.T) {
	emptyDir := t.TempDir()

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Pack(context.Background(), emptyDir, oci.PackOptions{})
	if err == nil {
		t.Fatal("expected error for missing skill.yaml")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/oci/ -v`
Expected: compilation error — package does not exist

- [ ] **Step 3: Implement Client and annotations**

Create `pkg/oci/client.go`:

```go
package oci

import (
	"fmt"

	"oras.land/oras-go/v2/content/oci"
)

type Client struct {
	store     *oci.Store
	storePath string
}

func NewClient(storePath string) (*Client, error) {
	store, err := oci.New(storePath)
	if err != nil {
		return nil, fmt.Errorf("opening OCI store at %s: %w", storePath, err)
	}
	return &Client{store: store, storePath: storePath}, nil
}

type PackOptions struct {
	Tag string
}

type LocalImage struct {
	Name    string
	Version string
	Tag     string
	Digest  string
	Status  string
	Created string
}
```

Create `pkg/oci/annotations.go`:

```go
package oci

import (
	"fmt"
	"strings"
	"time"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func annotationsFromSkillCard(sc *skillcard.SkillCard) map[string]string {
	a := map[string]string{
		ocispec.AnnotationVersion: sc.Metadata.Version,
		ocispec.AnnotationCreated: time.Now().UTC().Format(time.RFC3339),
		lifecycle.StatusAnnotation: string(lifecycle.Draft),
	}
	if sc.Metadata.DisplayName != "" {
		a[ocispec.AnnotationTitle] = sc.Metadata.DisplayName
	}
	if sc.Metadata.Description != "" {
		desc := sc.Metadata.Description
		if len(desc) > 256 {
			desc = desc[:256]
		}
		a[ocispec.AnnotationDescription] = desc
	}
	if len(sc.Metadata.Authors) > 0 {
		var parts []string
		for _, author := range sc.Metadata.Authors {
			if author.Email != "" {
				parts = append(parts, fmt.Sprintf("%s <%s>", author.Name, author.Email))
			} else {
				parts = append(parts, author.Name)
			}
		}
		a[ocispec.AnnotationAuthors] = strings.Join(parts, ", ")
	}
	if sc.Metadata.License != "" {
		a[ocispec.AnnotationLicenses] = sc.Metadata.License
	}
	if sc.Metadata.Namespace != "" {
		a[ocispec.AnnotationVendor] = sc.Metadata.Namespace
	}
	if sc.Provenance != nil {
		if sc.Provenance.Source != "" {
			a[ocispec.AnnotationSource] = sc.Provenance.Source
		}
		if sc.Provenance.Commit != "" {
			a[ocispec.AnnotationRevision] = sc.Provenance.Commit
		}
	}
	return a
}
```

- [ ] **Step 4: Implement Pack**

Create `pkg/oci/pack.go`:

```go
package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
	"github.com/redhat-et/oci-skill-registry/pkg/skillcard"
	"oras.land/oras-go/v2"
)

func (c *Client) Pack(ctx context.Context, dir string, opts PackOptions) (ocispec.Descriptor, error) {
	skillPath := filepath.Join(dir, "skill.yaml")
	f, err := os.Open(skillPath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("opening skill.yaml: %w", err)
	}
	defer f.Close()

	sc, err := skillcard.Parse(f)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing skill.yaml: %w", err)
	}

	errs, err := skillcard.Validate(sc)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("validating skill.yaml: %w", err)
	}
	if len(errs) > 0 {
		return ocispec.Descriptor{}, fmt.Errorf("skill.yaml validation failed: %v", errs)
	}

	layerData, uncompressedDigest, err := createLayer(dir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("creating layer: %w", err)
	}

	layerDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayerGzip,
		Digest:    digestBytes(layerData),
		Size:      int64(len(layerData)),
	}
	if err := c.store.Push(ctx, layerDesc, bytes.NewReader(layerData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing layer: %w", err)
	}

	imageConfig := ocispec.Image{
		Architecture: "amd64",
		OS:           "linux",
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []ocispec.Digest{uncompressedDigest},
		},
	}
	configData, err := json.Marshal(imageConfig)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling config: %w", err)
	}

	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digestBytes(configData),
		Size:      int64(len(configData)),
	}
	if err := c.store.Push(ctx, configDesc, bytes.NewReader(configData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing config: %w", err)
	}

	annotations := annotationsFromSkillCard(sc)

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{layerDesc},
		Annotations: annotations,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digestBytes(manifestData),
		Size:      int64(len(manifestData)),
	}
	if err := c.store.Push(ctx, manifestDesc, bytes.NewReader(manifestData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing manifest: %w", err)
	}

	tag := opts.Tag
	if tag == "" {
		tag = lifecycle.TagForState(sc.Metadata.Version, lifecycle.Draft)
	}
	ref := sc.Metadata.Namespace + "/" + sc.Metadata.Name + ":" + tag
	if err := c.store.Tag(ctx, manifestDesc, ref); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tagging: %w", err)
	}

	return manifestDesc, nil
}

func createLayer(dir string) ([]byte, ocispec.Digest, error) {
	var compressed bytes.Buffer
	gzWriter := gzip.NewWriter(&compressed)

	var uncompressed bytes.Buffer
	multiTar := io.MultiWriter(gzWriter, &uncompressed)
	tarWriter := tar.NewWriter(multiTar)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip hidden directories
		if d.IsDir() && rel[0] == '.' {
			return filepath.SkipDir
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !d.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tarWriter, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("walking directory: %w", err)
	}

	if err := tarWriter.Close(); err != nil {
		return nil, "", err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, "", err
	}

	uncompressedDigest := digestBytes(uncompressed.Bytes())
	return compressed.Bytes(), uncompressedDigest, nil
}

func digestBytes(data []byte) ocispec.Digest {
	h := sha256.Sum256(data)
	return ocispec.Digest(fmt.Sprintf("sha256:%x", h))
}

func (c *Client) ListLocal() ([]LocalImage, error) {
	ctx := context.Background()
	var images []LocalImage

	if err := c.store.Tags(ctx, "", func(tags []string) error {
		for _, tag := range tags {
			desc, err := c.store.Resolve(ctx, tag)
			if err != nil {
				continue
			}
			rc, err := c.store.Fetch(ctx, desc)
			if err != nil {
				continue
			}
			manifestData, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			var manifest ocispec.Manifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				continue
			}

			name := tag
			version := manifest.Annotations[ocispec.AnnotationVersion]
			status := manifest.Annotations[lifecycle.StatusAnnotation]
			created := manifest.Annotations[ocispec.AnnotationCreated]

			// Extract name from tag (namespace/name:tag format)
			if idx := strings.LastIndex(tag, ":"); idx > 0 {
				name = tag[:idx]
			}

			images = append(images, LocalImage{
				Name:    name,
				Version: version,
				Tag:     tag,
				Digest:  desc.Digest.String(),
				Status:  status,
				Created: created,
			})
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}

	return images, nil
}
```

Note: add `"strings"` to the import block above.

- [ ] **Step 5: Check for compilation issues**

Run: `go build ./pkg/oci/`

If there are import issues with ocispec.Digest vs digest.Digest,
adjust the type. oras-go uses `github.com/opencontainers/go-digest`
for digest types. The `ocispec.Digest` type may actually be
`digest.Digest` from that package. Update imports accordingly:

```go
import "github.com/opencontainers/go-digest"
```

And replace `ocispec.Digest` with `digest.Digest` where needed.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./pkg/oci/ -v`
Expected: all tests PASS

- [ ] **Step 7: Run linter**

Run: `golangci-lint run ./pkg/oci/`
Expected: no issues

- [ ] **Step 8: Commit**

```bash
git add pkg/oci/
git commit -s -m "feat: add OCI client with Pack and ListLocal"
```

---

## Task 7: skillctl pack and skillctl images

**Files:**

- Create: `internal/cli/pack.go`
- Create: `internal/cli/images.go`
- Modify: `internal/cli/root.go` (add commands)

- [ ] **Step 1: Create pack command**

Create `internal/cli/pack.go`:

```go
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
	"github.com/spf13/cobra"
)

func newPackCmd() *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "pack <dir>",
		Short: "Pack a skill directory into a local OCI image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPack(cmd, args[0], tag)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "override the image tag (default: <version>-draft)")
	return cmd
}

func runPack(cmd *cobra.Command, dir string, tag string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Pack(context.Background(), dir, oci.PackOptions{Tag: tag})
	if err != nil {
		return fmt.Errorf("packing %s: %w", dir, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Packed %s\nDigest: %s\n", dir, desc.Digest)
	return nil
}

func defaultClient() (*oci.Client, error) {
	storeDir, err := defaultStoreDir()
	if err != nil {
		return nil, err
	}
	return oci.NewClient(storeDir)
}

func defaultStoreDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding home directory: %w", err)
		}
		dataDir = home + "/.local/share"
	}
	dir := dataDir + "/skillctl/store"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating store directory: %w", err)
	}
	return dir, nil
}
```

- [ ] **Step 2: Create images command**

Create `internal/cli/images.go`:

```go
package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List skill images in local store",
		Args:  cobra.NoArgs,
		RunE:  runImages,
	}
}

func runImages(cmd *cobra.Command, args []string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	images, err := client.ListLocal()
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	if len(images) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No images found in local store.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tDIGEST\tCREATED")
	for _, img := range images {
		shortDigest := img.Digest
		if len(shortDigest) > 19 {
			shortDigest = shortDigest[:19]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			img.Name, img.Version, img.Status, shortDigest, img.Created)
	}
	return w.Flush()
}
```

- [ ] **Step 3: Register commands in root**

Update `internal/cli/root.go` — add to `NewRootCmd` before
`return cmd`:

```go
cmd.AddCommand(newPackCmd())
cmd.AddCommand(newImagesCmd())
```

- [ ] **Step 4: Build and test**

Run: `go build -o bin/skillctl ./cmd/skillctl`

Run: `./bin/skillctl pack examples/hello-world/`
Expected: `Packed examples/hello-world/` with a digest

Run: `./bin/skillctl images`
Expected: table showing `examples/hello-world` with version
`1.0.0`, status `draft`, and a digest

- [ ] **Step 5: Commit**

```bash
git add internal/cli/pack.go internal/cli/images.go internal/cli/root.go
git commit -s -m "feat: add skillctl pack and images commands"
```

---

## Task 8: OCI Push and Pull

**Files:**

- Create: `pkg/oci/push.go`
- Create: `pkg/oci/pull.go`
- Modify: `pkg/oci/client.go` (add PushOptions, PullOptions)
- Modify: `pkg/oci/oci_test.go` (add push/pull tests)

- [ ] **Step 1: Add Push/Pull option types to client.go**

Add to `pkg/oci/client.go`:

```go
type PushOptions struct{}

type PullOptions struct {
	OutputDir string
}
```

- [ ] **Step 2: Write push/pull tests**

Append to `pkg/oci/oci_test.go`:

```go
func TestPushAndPull(t *testing.T) {
	// Set up source store and pack a skill
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	srcStoreDir := t.TempDir()
	srcClient, err := oci.NewClient(srcStoreDir)
	if err != nil {
		t.Fatalf("NewClient (src): %v", err)
	}

	ctx := context.Background()
	_, err = srcClient.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Set up destination store (simulates remote registry)
	dstStoreDir := t.TempDir()
	dstClient, err := oci.NewClient(dstStoreDir)
	if err != nil {
		t.Fatalf("NewClient (dst): %v", err)
	}

	// Push from src to dst using CopyTo
	ref := "test/test-skill:1.0.0-draft"
	err = srcClient.CopyTo(ctx, ref, dstClient)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	// Pull from dst to a new store
	pullStoreDir := t.TempDir()
	pullClient, err := oci.NewClient(pullStoreDir)
	if err != nil {
		t.Fatalf("NewClient (pull): %v", err)
	}

	err = dstClient.CopyTo(ctx, ref, pullClient)
	if err != nil {
		t.Fatalf("CopyTo (pull): %v", err)
	}

	// Verify the pulled image exists
	images, err := pullClient.ListLocal()
	if err != nil {
		t.Fatalf("ListLocal: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image after pull, got %d", len(images))
	}
}

func TestPullWithUnpack(t *testing.T) {
	// Pack a skill
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Unpack to output directory
	outputDir := t.TempDir()
	err = client.Unpack(ctx, "test/test-skill:1.0.0-draft", outputDir)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	// Verify unpacked files
	expectedFile := filepath.Join(outputDir, "test-skill", "skill.yaml")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected %s to exist: %v", expectedFile, err)
	}

	promptFile := filepath.Join(outputDir, "test-skill", "SKILL.md")
	if _, err := os.Stat(promptFile); err != nil {
		t.Errorf("expected %s to exist: %v", promptFile, err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./pkg/oci/ -run "TestPush|TestPull" -v`
Expected: compilation error — `CopyTo` and `Unpack` not defined

- [ ] **Step 4: Implement Push (CopyTo)**

Create `pkg/oci/push.go`:

```go
package oci

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2"
)

func (c *Client) Push(ctx context.Context, ref string, opts PushOptions) error {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return fmt.Errorf("resolving remote: %w", err)
	}

	tag := tagFromRef(ref)
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving local ref %s: %w", ref, err)
	}

	if _, err := oras.Copy(ctx, c.store, desc.Digest.String(), repo, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("pushing to %s: %w", ref, err)
	}

	return nil
}

// CopyTo copies an image from this store to another Client's store.
// Used for testing without a real registry.
func (c *Client) CopyTo(ctx context.Context, ref string, dst *Client) error {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	tag := tagFromRef(ref)
	if _, err := oras.Copy(ctx, c.store, desc.Digest.String(), dst.store, tag, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("copying %s: %w", ref, err)
	}

	return nil
}

func tagFromRef(ref string) string {
	if idx := lastIndex(ref, ':'); idx >= 0 {
		return ref[idx+1:]
	}
	return "latest"
}

func lastIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 5: Implement Pull and Unpack**

Create `pkg/oci/pull.go`:

```go
package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry/remote"
)

func (c *Client) Pull(ctx context.Context, ref string, opts PullOptions) (ocispec.Descriptor, error) {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("resolving remote: %w", err)
	}

	tag := tagFromRef(ref)
	desc, err := oras.Copy(ctx, repo, tag, c.store, tag, oras.DefaultCopyOptions)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pulling %s: %w", ref, err)
	}

	if opts.OutputDir != "" {
		if err := c.Unpack(ctx, tag, opts.OutputDir); err != nil {
			return desc, fmt.Errorf("unpacking: %w", err)
		}
	}

	return desc, nil
}

func (c *Client) Unpack(ctx context.Context, ref string, outputDir string) error {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetching manifest: %w", err)
	}
	manifestData, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Determine skill name from ref or annotations
	skillName := ref
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		skillName = ref[:idx]
	}
	if idx := strings.LastIndex(skillName, "/"); idx >= 0 {
		skillName = skillName[idx+1:]
	}

	targetDir := filepath.Join(outputDir, skillName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	for _, layerDesc := range manifest.Layers {
		if err := c.extractLayer(ctx, layerDesc, targetDir); err != nil {
			return fmt.Errorf("extracting layer: %w", err)
		}
	}

	return nil
}

func (c *Client) extractLayer(ctx context.Context, desc ocispec.Descriptor, targetDir string) error {
	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetching layer: %w", err)
	}
	defer rc.Close()

	gzReader, err := gzip.NewReader(rc)
	if err != nil {
		return fmt.Errorf("decompressing layer: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		target := filepath.Join(targetDir, header.Name)
		if !strings.HasPrefix(target, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid tar path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tarReader); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func newRemoteRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("creating remote repository for %s: %w", ref, err)
	}
	return repo, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./pkg/oci/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add pkg/oci/
git commit -s -m "feat: add OCI Push, Pull, and Unpack"
```

---

## Task 9: skillctl push and skillctl pull

**Files:**

- Create: `internal/cli/push.go`
- Create: `internal/cli/pull.go`
- Modify: `internal/cli/root.go` (add commands)

- [ ] **Step 1: Create push command**

Create `internal/cli/push.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <ref>",
		Short: "Push a skill image from local store to a remote registry",
		Long: `Push a skill image to a remote OCI registry.

The ref should be a full OCI reference: registry/namespace/name:tag
Example: quay.io/acme/hr-onboarding:1.0.0-draft`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd, args[0])
		},
	}
}

func runPush(cmd *cobra.Command, ref string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	if err := client.Push(context.Background(), ref, oci.PushOptions{}); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed %s\n", ref)
	return nil
}
```

- [ ] **Step 2: Create pull command**

Create `internal/cli/pull.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/oci-skill-registry/pkg/oci"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	var outputDir string
	cmd := &cobra.Command{
		Use:   "pull <ref>",
		Short: "Pull a skill image from a remote registry",
		Long: `Pull a skill image from an OCI registry to the local store.

Use -o to unpack skill files to a directory. If -o points to an
existing directory, a subdirectory named after the skill is created
automatically.

Examples:
  skillctl pull quay.io/acme/hr-onboarding:1.0.0
  skillctl pull quay.io/acme/hr-onboarding:1.0.0 -o ./skills/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args[0], outputDir)
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "unpack skill files to directory")
	return cmd
}

func runPull(cmd *cobra.Command, ref string, outputDir string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	desc, err := client.Pull(context.Background(), ref, oci.PullOptions{
		OutputDir: outputDir,
	})
	if err != nil {
		return fmt.Errorf("pulling: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled %s\nDigest: %s\n", ref, desc.Digest)
	if outputDir != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Unpacked to %s\n", outputDir)
	}
	return nil
}
```

- [ ] **Step 3: Register commands in root**

Update `internal/cli/root.go` — add to `NewRootCmd`:

```go
cmd.AddCommand(newPushCmd())
cmd.AddCommand(newPullCmd())
```

- [ ] **Step 4: Build and verify help text**

Run: `go build -o bin/skillctl ./cmd/skillctl`
Run: `./bin/skillctl push --help`
Run: `./bin/skillctl pull --help`
Expected: help text displays correctly

- [ ] **Step 5: Commit**

```bash
git add internal/cli/push.go internal/cli/pull.go internal/cli/root.go
git commit -s -m "feat: add skillctl push and pull commands"
```

---

## Task 10: OCI Inspect and skillctl inspect

**Files:**

- Create: `pkg/oci/inspect.go`
- Create: `internal/cli/inspect.go`
- Modify: `pkg/oci/client.go` (add InspectResult type)
- Modify: `pkg/oci/oci_test.go` (add inspect tests)
- Modify: `internal/cli/root.go` (add command)

- [ ] **Step 1: Add InspectResult to client.go**

Add to `pkg/oci/client.go`:

```go
type InspectResult struct {
	Name        string
	DisplayName string
	Version     string
	Status      string
	Description string
	Authors     string
	License     string
	Tags        string
	Digest      string
	Created     string
	MediaType   string
	Size        int64
	LayerCount  int
}
```

- [ ] **Step 2: Write inspect tests**

Append to `pkg/oci/oci_test.go`:

```go
func TestInspect(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-draft")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", result.Version, "1.0.0")
	}
	if result.Status != "draft" {
		t.Errorf("status = %q, want %q", result.Status, "draft")
	}
	if result.Name != "test/test-skill" {
		t.Errorf("name = %q, want %q", result.Name, "test/test-skill")
	}
	if result.LayerCount != 1 {
		t.Errorf("layer count = %d, want 1", result.LayerCount)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/oci/ -run TestInspect -v`
Expected: compilation error — `Inspect` not defined

- [ ] **Step 4: Implement Inspect**

Create `pkg/oci/inspect.go`:

```go
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

func (c *Client) Inspect(ctx context.Context, ref string) (*InspectResult, error) {
	desc, err := c.store.Resolve(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := c.store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching manifest: %w", err)
	}
	manifestData, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Extract name from ref
	name := ref
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		name = ref[:idx]
	}

	var totalSize int64
	for _, l := range manifest.Layers {
		totalSize += l.Size
	}

	result := &InspectResult{
		Name:        name,
		DisplayName: manifest.Annotations[ocispec.AnnotationTitle],
		Version:     manifest.Annotations[ocispec.AnnotationVersion],
		Status:      manifest.Annotations[lifecycle.StatusAnnotation],
		Description: manifest.Annotations[ocispec.AnnotationDescription],
		Authors:     manifest.Annotations[ocispec.AnnotationAuthors],
		License:     manifest.Annotations[ocispec.AnnotationLicenses],
		Digest:      desc.Digest.String(),
		Created:     manifest.Annotations[ocispec.AnnotationCreated],
		MediaType:   string(desc.MediaType),
		Size:        totalSize,
		LayerCount:  len(manifest.Layers),
	}

	return result, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/oci/ -run TestInspect -v`
Expected: PASS

- [ ] **Step 6: Create inspect CLI command**

Create `internal/cli/inspect.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Show SkillCard metadata and OCI image details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd, args[0])
		},
	}
}

func runInspect(cmd *cobra.Command, ref string) error {
	client, err := defaultClient()
	if err != nil {
		return err
	}

	result, err := client.Inspect(context.Background(), ref)
	if err != nil {
		return fmt.Errorf("inspecting %s: %w", ref, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Skill:        %s\n", result.Name)
	if result.DisplayName != "" {
		fmt.Fprintf(out, "Display Name: %s\n", result.DisplayName)
	}
	fmt.Fprintf(out, "Version:      %s\n", result.Version)
	fmt.Fprintf(out, "Status:       %s\n", result.Status)
	if result.Description != "" {
		fmt.Fprintf(out, "Description:  %s\n", result.Description)
	}
	if result.Authors != "" {
		fmt.Fprintf(out, "Authors:      %s\n", result.Authors)
	}
	if result.License != "" {
		fmt.Fprintf(out, "License:      %s\n", result.License)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "OCI Image:\n")
	fmt.Fprintf(out, "  Digest:     %s\n", result.Digest)
	fmt.Fprintf(out, "  Created:    %s\n", result.Created)
	fmt.Fprintf(out, "  Size:       %d bytes\n", result.Size)
	fmt.Fprintf(out, "  Layers:     %d\n", result.LayerCount)

	return nil
}
```

- [ ] **Step 7: Register inspect in root.go**

Add to `NewRootCmd` in `internal/cli/root.go`:

```go
cmd.AddCommand(newInspectCmd())
```

- [ ] **Step 8: Build and test end-to-end**

Run: `go build -o bin/skillctl ./cmd/skillctl`
Run: `./bin/skillctl pack examples/hello-world/`
Run: `./bin/skillctl inspect examples/hello-world:1.0.0-draft`
Expected: formatted output showing skill metadata and OCI details

- [ ] **Step 9: Commit**

```bash
git add pkg/oci/inspect.go pkg/oci/client.go pkg/oci/oci_test.go \
  internal/cli/inspect.go internal/cli/root.go
git commit -s -m "feat: add skillctl inspect command"
```

---

## Task 11: OCI Promote and skillctl promote

**Files:**

- Create: `pkg/oci/promote.go`
- Create: `internal/cli/promote.go`
- Modify: `pkg/oci/oci_test.go` (add promote tests)
- Modify: `internal/cli/root.go` (add command)

- [ ] **Step 1: Write promote tests**

Append to `pkg/oci/oci_test.go`:

```go
func TestPromote(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Promote draft -> testing
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Testing)
	if err != nil {
		t.Fatalf("Promote to testing: %v", err)
	}

	// Verify new tag exists
	result, err := client.Inspect(ctx, "test/test-skill:1.0.0-rc")
	if err != nil {
		t.Fatalf("Inspect after promote: %v", err)
	}
	if result.Status != "testing" {
		t.Errorf("status = %q, want %q", result.Status, "testing")
	}

	// Promote testing -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-rc", lifecycle.Published)
	if err != nil {
		t.Fatalf("Promote to published: %v", err)
	}

	result, err = client.Inspect(ctx, "test/test-skill:1.0.0")
	if err != nil {
		t.Fatalf("Inspect after publish: %v", err)
	}
	if result.Status != "published" {
		t.Errorf("status = %q, want %q", result.Status, "published")
	}
}

func TestPromoteInvalidTransition(t *testing.T) {
	skillDir := t.TempDir()
	writeTestSkill(t, skillDir)

	storeDir := t.TempDir()
	client, err := oci.NewClient(storeDir)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	_, err = client.Pack(ctx, skillDir, oci.PackOptions{})
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Try invalid transition: draft -> published
	err = client.PromoteLocal(ctx, "test/test-skill:1.0.0-draft", lifecycle.Published)
	if err == nil {
		t.Fatal("expected error for invalid transition draft -> published")
	}
}
```

Add this import at the top of the test file:

```go
"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/oci/ -run TestPromote -v`
Expected: compilation error — `PromoteLocal` not defined

- [ ] **Step 3: Implement Promote**

Create `pkg/oci/promote.go`:

```go
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
)

// PromoteLocal promotes a skill image within the local store.
// Used for testing and local workflows.
func (c *Client) PromoteLocal(ctx context.Context, ref string, to lifecycle.State) error {
	return c.promote(ctx, c.store, ref, to)
}

type promotableStore interface {
	Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error)
	Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error)
	Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error
	Tag(ctx context.Context, desc ocispec.Descriptor, ref string) error
}

func (c *Client) promote(ctx context.Context, store promotableStore, ref string, to lifecycle.State) error {
	desc, err := store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", ref, err)
	}

	rc, err := store.Fetch(ctx, desc)
	if err != nil {
		return fmt.Errorf("fetching manifest: %w", err)
	}
	manifestData, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	currentStatus := manifest.Annotations[lifecycle.StatusAnnotation]
	from, err := lifecycle.ParseState(currentStatus)
	if err != nil {
		return fmt.Errorf("current state invalid: %w", err)
	}

	if !lifecycle.ValidTransition(from, to) {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}

	manifest.Annotations[lifecycle.StatusAnnotation] = string(to)

	newManifestData, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling updated manifest: %w", err)
	}

	newDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digestBytes(newManifestData),
		Size:      int64(len(newManifestData)),
	}

	if err := store.Push(ctx, newDesc, bytes.NewReader(newManifestData)); err != nil {
		return fmt.Errorf("pushing updated manifest: %w", err)
	}

	// Extract namespace/name from ref
	name := ref
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		name = ref[:idx]
	}

	version := manifest.Annotations[ocispec.AnnotationVersion]
	newTag := lifecycle.TagForState(version, to)
	if newTag != "" {
		fullRef := name + ":" + newTag
		if err := store.Tag(ctx, newDesc, fullRef); err != nil {
			return fmt.Errorf("tagging %s: %w", fullRef, err)
		}
	}

	// Tag as "latest" when publishing
	if to == lifecycle.Published {
		latestRef := name + ":latest"
		if err := store.Tag(ctx, newDesc, latestRef); err != nil {
			return fmt.Errorf("tagging latest: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/oci/ -v`
Expected: all tests PASS

- [ ] **Step 5: Create promote CLI command**

Create `internal/cli/promote.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/redhat-et/oci-skill-registry/pkg/lifecycle"
	"github.com/redhat-et/oci-skill-registry/pkg/oci"
	"github.com/spf13/cobra"
)

func newPromoteCmd() *cobra.Command {
	var toState string
	var local bool
	cmd := &cobra.Command{
		Use:   "promote <ref>",
		Short: "Promote a skill to the next lifecycle state",
		Long: `Promote a skill image to a new lifecycle state.

State transitions: draft -> testing -> published -> deprecated -> archived

By default, operates on a remote registry. Use --local to promote
images in the local store.

Examples:
  skillctl promote quay.io/acme/hr-onboarding:1.0.0-draft --to testing
  skillctl promote test/test-skill:1.0.0-draft --to testing --local`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromote(cmd, args[0], toState, local)
		},
	}
	cmd.Flags().StringVar(&toState, "to", "", "target lifecycle state (required)")
	cmd.MarkFlagRequired("to")
	cmd.Flags().BoolVar(&local, "local", false, "promote in local store instead of remote registry")
	return cmd
}

func runPromote(cmd *cobra.Command, ref string, toState string, local bool) error {
	to, err := lifecycle.ParseState(toState)
	if err != nil {
		return fmt.Errorf("invalid state %q: %w", toState, err)
	}

	client, err := defaultClient()
	if err != nil {
		return err
	}

	if local {
		if err := client.PromoteLocal(context.Background(), ref, to); err != nil {
			return fmt.Errorf("promoting: %w", err)
		}
	} else {
		if err := client.Promote(context.Background(), ref, to, oci.PromoteOptions{}); err != nil {
			return fmt.Errorf("promoting: %w", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Promoted %s -> %s\n", ref, to)
	return nil
}
```

- [ ] **Step 6: Add remote Promote stub to client**

Add to `pkg/oci/client.go`:

```go
type PromoteOptions struct{}
```

Add to `pkg/oci/promote.go`:

```go
// Promote promotes a skill on a remote registry.
func (c *Client) Promote(ctx context.Context, ref string, to lifecycle.State, opts PromoteOptions) error {
	repo, err := newRemoteRepository(ref)
	if err != nil {
		return fmt.Errorf("resolving remote: %w", err)
	}
	return c.promote(ctx, repo, ref, to)
}
```

- [ ] **Step 7: Register promote in root.go**

Add to `NewRootCmd` in `internal/cli/root.go`:

```go
cmd.AddCommand(newPromoteCmd())
```

- [ ] **Step 8: Build and test end-to-end**

Run: `go build -o bin/skillctl ./cmd/skillctl`
Run: `./bin/skillctl pack examples/hello-world/`
Run: `./bin/skillctl promote examples/hello-world:1.0.0-draft --to testing --local`
Expected: `Promoted examples/hello-world:1.0.0-draft -> testing`
Run: `./bin/skillctl inspect examples/hello-world:1.0.0-rc`
Expected: status shows `testing`

- [ ] **Step 9: Run full test suite and linter**

Run: `go test ./... -v`
Expected: all tests PASS

Run: `golangci-lint run`
Expected: no issues (or minor issues to fix)

- [ ] **Step 10: Commit**

```bash
git add pkg/oci/ internal/cli/
git commit -s -m "feat: add skillctl promote with lifecycle state machine"
```

---

## Final verification

After all tasks are complete:

- [ ] **Run full test suite:** `make test`
- [ ] **Run linter:** `make lint`
- [ ] **Build:** `make build`
- [ ] **Smoke test the full workflow:**

```bash
./bin/skillctl validate examples/hello-world/
./bin/skillctl pack examples/hello-world/
./bin/skillctl images
./bin/skillctl inspect examples/hello-world:1.0.0-draft
./bin/skillctl promote examples/hello-world:1.0.0-draft --to testing --local
./bin/skillctl inspect examples/hello-world:1.0.0-rc
./bin/skillctl promote examples/hello-world:1.0.0-rc --to published --local
./bin/skillctl inspect examples/hello-world:1.0.0
./bin/skillctl images
```

Expected: all commands succeed, lifecycle progression works,
images list shows the skill with updated status and tags.
