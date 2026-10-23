// Package agentplugin validates the portable Agent Plugins v1 package format.
package agentplugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	ManifestSchemaV1 = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	MCPSchemaV1      = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"
	SpecVersionV1    = "1.0.0"
)

// Manifest is the portable metadata from the root plugin.json.
type Manifest struct {
	Schema      string         `json:"$schema"`
	Name        string         `json:"name"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Author      *Author        `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Extensions  map[string]any `json:"extensions,omitempty"`
}

// Author is the optional Agent Plugins manifest author object.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Info describes a validated plugin package and its discovered v1 components.
type Info struct {
	Root       string
	Manifest   Manifest
	Skills     []string
	MCPServers []string
	Extensions []string
	FileCount  int
}

// ValidateDir validates a plugin directory against Agent Plugins 1.0.0 and
// the Agent Skills format used by skills discovered under its root skills/.
func ValidateDir(dir string) (*Info, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving plugin directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolving plugin directory: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("accessing plugin directory: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("plugin path %s is not a directory", dir)
	}

	fileCount, err := validateTree(root)
	if err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(root, "plugin.json")
	manifestBytes, err := readContainedRegularFile(root, manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading plugin.json: %w", err)
	}
	manifest, err := parseManifest(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("validating plugin.json: %w", err)
	}

	info := &Info{Root: root, Manifest: manifest, FileCount: fileCount}
	info.Extensions = make([]string, 0, len(manifest.Extensions))
	for namespace := range manifest.Extensions {
		info.Extensions = append(info.Extensions, namespace)
	}
	sort.Strings(info.Extensions)

	skillsPath := filepath.Join(root, "skills")
	if st, err := os.Stat(skillsPath); err == nil {
		if !st.IsDir() {
			return nil, fmt.Errorf("skills must be a directory when present")
		}
		entries, err := os.ReadDir(skillsPath)
		if err != nil {
			return nil, fmt.Errorf("reading skills directory: %w", err)
		}
		for _, entry := range entries {
			childPath := filepath.Join(skillsPath, entry.Name())
			childInfo, err := os.Stat(childPath)
			if err != nil {
				return nil, fmt.Errorf("checking skill entry %s: %w", entry.Name(), err)
			}
			if !childInfo.IsDir() {
				continue
			}
			skillPath := filepath.Join(childPath, "SKILL.md")
			if _, err := os.Lstat(skillPath); errorsIsNotExist(err) {
				continue
			} else if err != nil {
				return nil, fmt.Errorf("checking %s/SKILL.md: %w", entry.Name(), err)
			}
			skillBytes, err := readContainedRegularFile(root, skillPath)
			if err != nil {
				return nil, fmt.Errorf("validating skill %s: %w", entry.Name(), err)
			}
			if err := validateSkillMD(skillBytes, entry.Name()); err != nil {
				return nil, fmt.Errorf("validating skill %s: %w", entry.Name(), err)
			}
			info.Skills = append(info.Skills, entry.Name())
		}
	} else if !errorsIsNotExist(err) {
		return nil, fmt.Errorf("checking skills directory: %w", err)
	}
	sort.Strings(info.Skills)

	mcpPath := filepath.Join(root, "mcp.json")
	if _, err := os.Lstat(mcpPath); err == nil {
		mcpBytes, err := readContainedRegularFile(root, mcpPath)
		if err != nil {
			return nil, fmt.Errorf("reading mcp.json: %w", err)
		}
		servers, err := validateMCP(mcpBytes)
		if err != nil {
			return nil, fmt.Errorf("validating mcp.json: %w", err)
		}
		info.MCPServers = servers
	} else if !errorsIsNotExist(err) {
		return nil, fmt.Errorf("checking mcp.json: %w", err)
	}

	return info, nil
}

func parseManifest(data []byte) (Manifest, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return Manifest{}, err
	}
	if fields == nil {
		return Manifest{}, fmt.Errorf("must be a JSON object")
	}
	allowed := map[string]bool{
		"$schema": true, "name": true, "version": true, "description": true,
		"author": true, "homepage": true, "repository": true, "license": true,
		"keywords": true, "extensions": true,
	}
	for key := range fields {
		if !allowed[key] {
			return Manifest{}, fmt.Errorf("unknown top-level field %q; client-specific data belongs under extensions", key)
		}
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Schema != ManifestSchemaV1 {
		return Manifest{}, fmt.Errorf("unsupported or missing $schema %q (supported: %s)", manifest.Schema, ManifestSchemaV1)
	}
	if !validPluginName(manifest.Name) {
		return Manifest{}, fmt.Errorf("name must be 1-64 lowercase ASCII letters, digits, hyphens, or periods, start and end with a letter or digit, and contain no consecutive hyphens or periods")
	}
	for _, key := range []string{"version", "description", "homepage", "repository", "license"} {
		if raw, ok := fields[key]; ok && !isJSONType(raw, '"') {
			return Manifest{}, fmt.Errorf("%s must be a string", key)
		}
	}
	if raw, ok := fields["author"]; ok {
		var author map[string]json.RawMessage
		if err := json.Unmarshal(raw, &author); err != nil || author == nil {
			return Manifest{}, fmt.Errorf("author must be an object")
		}
		for key, value := range author {
			if key != "name" && key != "email" && key != "url" {
				return Manifest{}, fmt.Errorf("author.%s is not a supported field", key)
			}
			if !isJSONType(value, '"') {
				return Manifest{}, fmt.Errorf("author.%s must be a string", key)
			}
		}
	}
	if raw, ok := fields["keywords"]; ok {
		if !isJSONType(raw, '[') {
			return Manifest{}, fmt.Errorf("keywords must be an array of strings")
		}
		var words []json.RawMessage
		if err := json.Unmarshal(raw, &words); err != nil {
			return Manifest{}, fmt.Errorf("keywords must be an array of strings")
		}
		for i, word := range words {
			if !isJSONType(word, '"') {
				return Manifest{}, fmt.Errorf("keywords[%d] must be a string", i)
			}
		}
	}
	if raw, ok := fields["extensions"]; ok {
		var extensions map[string]json.RawMessage
		if err := json.Unmarshal(raw, &extensions); err != nil || extensions == nil {
			return Manifest{}, fmt.Errorf("extensions must be an object")
		}
		for namespace, value := range extensions {
			if !validExtensionNamespace(namespace) {
				return Manifest{}, fmt.Errorf("extensions key %q must use a reverse-domain namespace", namespace)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(value, &object); err != nil || object == nil {
				return Manifest{}, fmt.Errorf("extensions.%s must be an object", namespace)
			}
		}
	}
	return manifest, nil
}

func validateTree(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if path != root && IsVCSMetadataEntry(filepath.Base(path)) {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		count++
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("reading symlink %s: %w", path, err)
			}
			if filepath.IsAbs(linkTarget) || filepath.VolumeName(linkTarget) != "" || runtime.GOOS != "windows" && strings.Contains(linkTarget, "\\") {
				return fmt.Errorf("symlink %s has an absolute or non-portable target", path)
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("symlink %s does not resolve: %w", filepath.ToSlash(strings.TrimPrefix(path, root+string(os.PathSeparator))), err)
			}
			if !withinRoot(root, resolved) {
				return fmt.Errorf("symlink %s resolves outside plugin root", filepath.ToSlash(strings.TrimPrefix(path, root+string(os.PathSeparator))))
			}
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported filesystem entry %s (%s)", path, info.Mode().Type())
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("scanning plugin directory: %w", err)
	}
	return count, nil
}

// IsVCSMetadataEntry reports whether name is a version-control metadata entry
// that should be excluded from plugin validation and packaging.
func IsVCSMetadataEntry(name string) bool {
	switch name {
	case ".git", ".hg", ".svn":
		return true
	default:
		return false
	}
}

func readContainedRegularFile(root, file string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return nil, err
	}
	if !withinRoot(root, resolved) {
		return nil, fmt.Errorf("path resolves outside plugin root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	return os.ReadFile(resolved)
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func validPluginName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, r := range name {
		valid := isLowercaseASCIILetterOrDigit(r) || r == '-' || r == '.'
		if !valid || (i == 0 || i == len(name)-1) && !isLowercaseASCIILetterOrDigit(r) {
			return false
		}
	}
	return !strings.Contains(name, "--") && !strings.Contains(name, "..")
}

func isLowercaseASCIILetterOrDigit(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func validExtensionNamespace(namespace string) bool {
	parts := strings.Split(namespace, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, r := range part {
			if !isLowercaseASCIILetterOrDigit(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

func isJSONType(raw json.RawMessage, first byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == first
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

func validateSkillMD(data []byte, directoryName string) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("SKILL.md is not valid UTF-8")
	}
	text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return fmt.Errorf("must begin with YAML frontmatter delimited by ---")
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return fmt.Errorf("frontmatter has no closing ---")
	}
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:closing], "\n")), &frontmatter); err != nil {
		return fmt.Errorf("parsing frontmatter: %w", err)
	}
	if frontmatter == nil {
		return fmt.Errorf("frontmatter must be a mapping")
	}
	name, ok := frontmatter["name"].(string)
	if !ok || !validSkillName(name) {
		return fmt.Errorf("frontmatter name must be a valid Agent Skill name")
	}
	if name != directoryName {
		return fmt.Errorf("frontmatter name %q must match its directory %q", name, directoryName)
	}
	description, ok := frontmatter["description"].(string)
	if !ok || strings.TrimSpace(description) == "" || len([]rune(description)) > 1024 {
		return fmt.Errorf("frontmatter description must be a non-empty string of at most 1024 characters")
	}
	for _, field := range []string{"license", "compatibility", "allowed-tools"} {
		if value, exists := frontmatter[field]; exists {
			stringValue, ok := value.(string)
			if !ok {
				return fmt.Errorf("frontmatter %s must be a string", field)
			}
			if field == "compatibility" && (len([]rune(stringValue)) < 1 || len([]rune(stringValue)) > 500) {
				return fmt.Errorf("frontmatter compatibility must be 1-500 characters")
			}
		}
	}
	if value, exists := frontmatter["metadata"]; exists {
		metadata, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("frontmatter metadata must be a mapping")
		}
		for key, item := range metadata {
			if key == "" {
				return fmt.Errorf("frontmatter metadata keys must be non-empty strings")
			}
			if _, ok := item.(string); !ok {
				return fmt.Errorf("frontmatter metadata.%s must be a string", key)
			}
		}
	}
	return nil
}

func validSkillName(name string) bool {
	if len(name) < 1 || len(name) > 64 || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if !isLowercaseASCIILetterOrDigit(r) && r != '-' {
			return false
		}
	}
	return true
}
