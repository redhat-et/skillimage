package agentplugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

func validateMCP(data []byte) ([]string, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	for key := range document {
		if key != "$schema" && key != "mcpServers" {
			return nil, fmt.Errorf("unknown top-level field %q", key)
		}
	}
	var schema string
	if err := json.Unmarshal(document["$schema"], &schema); err != nil || schema != MCPSchemaV1 {
		return nil, fmt.Errorf("$schema must be %q", MCPSchemaV1)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(document["mcpServers"], &servers); err != nil || servers == nil {
		return nil, fmt.Errorf("mcpServers must be an object")
	}

	names := make([]string, 0, len(servers))
	for name, raw := range servers {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("server names must not be empty")
		}
		if err := validateMCPServer(raw); err != nil {
			return nil, fmt.Errorf("server %q: %w", name, err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func validateMCPServer(raw json.RawMessage) error {
	var server map[string]json.RawMessage
	if err := json.Unmarshal(raw, &server); err != nil || server == nil {
		return fmt.Errorf("must be an object")
	}
	var transport string
	if err := json.Unmarshal(server["type"], &transport); err != nil {
		return fmt.Errorf("type must be a string")
	}
	allowed := map[string]bool{"type": true}
	switch transport {
	case "stdio":
		allowed["command"] = true
		allowed["args"] = true
		allowed["env"] = true
		allowed["cwd"] = true
	case "streamable-http", "sse":
		allowed["url"] = true
		allowed["headers"] = true
	default:
		return fmt.Errorf("type must be stdio, streamable-http, or sse")
	}
	for key := range server {
		if !allowed[key] {
			return fmt.Errorf("field %q is not valid for %s", key, transport)
		}
	}
	if transport == "stdio" {
		command, ok := jsonString(server["command"])
		if !ok || command == "" || strings.ContainsRune(command, '\x00') {
			return fmt.Errorf("command must be a non-empty executable token")
		}
		if len(strings.Fields(command)) != 1 || command != strings.TrimSpace(command) {
			return fmt.Errorf("command must contain one executable token")
		}
		if strings.HasPrefix(command, "./") {
			relativeCommand := command[2:]
			if filepath.Clean(filepath.FromSlash(relativeCommand)) == "." {
				return fmt.Errorf("command must name an executable inside the plugin root")
			}
			if err := validatePluginRelativePath(relativeCommand); err != nil {
				return fmt.Errorf("command: %w", err)
			}
		} else if command == "." || command == ".." || strings.ContainsAny(command, "/\\") {
			return fmt.Errorf("command must be a bare executable name or a path beginning with ./")
		}
		if rawArgs, ok := server["args"]; ok {
			var args []json.RawMessage
			if err := json.Unmarshal(rawArgs, &args); err != nil || args == nil && string(rawArgs) != "[]" {
				return fmt.Errorf("args must be an array of strings")
			}
			for i, arg := range args {
				if _, ok := jsonString(arg); !ok {
					return fmt.Errorf("args[%d] must be a string", i)
				}
			}
		}
		if rawEnv, ok := server["env"]; ok {
			var env map[string]json.RawMessage
			if err := json.Unmarshal(rawEnv, &env); err != nil || env == nil && string(rawEnv) != "{}" {
				return fmt.Errorf("env must be an object of strings")
			}
			for key, value := range env {
				if strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
					return fmt.Errorf("env.%s is reserved", key)
				}
				if _, ok := jsonString(value); !ok {
					return fmt.Errorf("env.%s must be a string", key)
				}
			}
		}
		if rawCWD, ok := server["cwd"]; ok {
			cwd, ok := jsonString(rawCWD)
			if !ok {
				return fmt.Errorf("cwd must be a string")
			}
			if err := validateCWD(cwd); err != nil {
				return err
			}
		}
		return nil
	}

	endpoint, ok := jsonString(server["url"])
	if !ok || endpoint == "" {
		return fmt.Errorf("url must be a non-empty string")
	}
	if err := validateEndpoint(endpoint); err != nil {
		return err
	}
	if rawHeaders, ok := server["headers"]; ok {
		var headers map[string]json.RawMessage
		if err := json.Unmarshal(rawHeaders, &headers); err != nil || headers == nil && string(rawHeaders) != "{}" {
			return fmt.Errorf("headers must be an object of strings")
		}
		seen := make(map[string]bool, len(headers))
		for name, value := range headers {
			if !validHeaderName(name) {
				return fmt.Errorf("header name %q is invalid", name)
			}
			lower := strings.ToLower(name)
			if seen[lower] {
				return fmt.Errorf("header %q is repeated with different casing", name)
			}
			seen[lower] = true
			text, ok := jsonString(value)
			if !ok || !validHeaderValue(text) {
				return fmt.Errorf("header %q must have a valid string value", name)
			}
		}
	}
	return nil
}

func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("url must be an absolute HTTP or HTTPS URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme must be http or https")
	}
	if u.User != nil || strings.Contains(raw, "#") {
		return fmt.Errorf("url must not contain user information or a fragment")
	}
	if scheme == "http" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("non-loopback MCP endpoints must use HTTPS")
		}
	}
	return nil
}

func validateCWD(cwd string) error {
	switch {
	case strings.HasPrefix(cwd, "./"):
		return validatePluginRelativePath(cwd[2:])
	case cwd == "${PLUGIN_ROOT}":
		return nil
	case strings.HasPrefix(cwd, "${PLUGIN_ROOT}/"):
		return validatePluginRelativePath(strings.TrimPrefix(cwd, "${PLUGIN_ROOT}/"))
	case cwd == "${PLUGIN_DATA}":
		return nil
	case strings.HasPrefix(cwd, "${PLUGIN_DATA}/"):
		return validateDataRelativePath(strings.TrimPrefix(cwd, "${PLUGIN_DATA}/"))
	default:
		return fmt.Errorf("cwd must start with ./, ${PLUGIN_ROOT}, or ${PLUGIN_DATA}")
	}
}

func validatePluginRelativePath(relative string) error {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || filepath.VolumeName(relative) != "" {
		if relative == "" {
			return nil
		}
		return fmt.Errorf("path must remain relative to the plugin root")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return fmt.Errorf("path escapes the plugin root")
	}
	return nil
}

func validateDataRelativePath(relative string) error {
	if relative == "" {
		return nil
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.Contains(relative, "\\") || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("cwd escapes PLUGIN_DATA")
	}
	return nil
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isHTTPTokenRune(r) {
			return false
		}
	}
	return true
}

func isHTTPTokenRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

func validHeaderValue(value string) bool {
	for _, b := range []byte(value) {
		if b < 0x20 && b != '\t' || b == 0x7f {
			return false
		}
	}
	return true
}

func jsonString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", false
	}
	var value string
	if json.Unmarshal(trimmed, &value) != nil {
		return "", false
	}
	return value, true
}
