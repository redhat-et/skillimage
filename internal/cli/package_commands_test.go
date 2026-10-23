package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginBuildInspectAndListCommands(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, command := range NewRootCmd("test").Commands() {
		if command.Name() == "plugin" {
			t.Fatal("plugin subcommand should be integrated into the general package commands")
		}
	}

	pluginDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", "incident-triage"))
	if err != nil {
		t.Fatalf("resolve plugin fixture: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err != nil {
		t.Fatalf("plugin fixture is unavailable: %v", err)
	}

	buildOutput, err := executeRootForTest(t, "build", pluginDir)
	if err != nil {
		t.Fatalf("build plugin command: %v", err)
	}
	if !strings.Contains(buildOutput, "Built plugin incident-triage:1.0.0") {
		t.Errorf("build output does not report the detected plugin and version: %s", buildOutput)
	}

	inspectOutput, err := executeRootForTest(t, "inspect", "incident-triage:1.0.0")
	if err != nil {
		t.Fatalf("inspect plugin command: %v", err)
	}
	for _, want := range []string{
		"Plugin:",
		"Agent Plugins 1.0.0",
		"MCP servers:",
		"incident-context",
		"com.example.skillctl",
	} {
		if !strings.Contains(inspectOutput, want) {
			t.Errorf("inspect output missing %q: %s", want, inspectOutput)
		}
	}

	listOutput, err := executeRootForTest(t, "list", "--kind", "plugin")
	if err != nil {
		t.Fatalf("list plugin command: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(listOutput), "\n")
	if len(lines) != 2 {
		t.Fatalf("plugin-only listing should contain a header and one row, got: %s", listOutput)
	}
	if !strings.Contains(lines[1], "plugin") || !strings.Contains(lines[1], "incident-triage") || !strings.Contains(lines[1], "1.0.0") {
		t.Errorf("plugin-only listing has the wrong row: %s", lines[1])
	}
}

func TestPluginBuildPassesTagAndMediaTypeToOCI(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	pluginDir, err := filepath.Abs(filepath.Join("..", "..", "examples", "plugins", "incident-triage"))
	if err != nil {
		t.Fatalf("resolve plugin fixture: %v", err)
	}

	buildOutput, err := executeRootForTest(t, "build", pluginDir, "--tag", "2.0.0")
	if err != nil {
		t.Fatalf("build plugin with tag override: %v", err)
	}
	if !strings.Contains(buildOutput, "Built plugin incident-triage:2.0.0") {
		t.Errorf("build output does not reflect the tag override: %s", buildOutput)
	}

	_, err = executeRootForTest(t, "build", pluginDir, "--media-type", "redhat")
	if err == nil || !strings.Contains(err.Error(), "agent plugin packages require standard OCI media types") {
		t.Fatalf("build plugin with redhat media type error = %v, want standard media type restriction", err)
	}
}

func executeRootForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd("test")
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}
