package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestMCPLaunchPolicy(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := db.MCPServer{Transport: "stdio", Command: command, Args: json.RawMessage(`["--headless"]`), Env: json.RawMessage(`{"MODE":"safe"}`)}
	t.Setenv("ARTEX_MCP_STDIO_ALLOWLIST", "")
	if validateMCPLaunch(&m) == nil {
		t.Fatal("missing policy allowed launch")
	}
	policy, _ := json.Marshal([]map[string]any{{"command": command, "args": []string{"--headless"}, "env": map[string]string{"MODE": "safe"}}})
	t.Setenv("ARTEX_MCP_STDIO_ALLOWLIST", string(policy))
	if err := validateMCPLaunch(&m); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*db.MCPServer)
	}{
		{"command", func(m *db.MCPServer) { m.Command = "sh" }},
		{"arguments", func(m *db.MCPServer) { m.Args = json.RawMessage(`["-c","echo injected"]`) }},
		{"environment", func(m *db.MCPServer) { m.Env = json.RawMessage(`{"MODE":"safe","NODE_OPTIONS":"--require=evil.js"}`) }},
		{"malformed arguments", func(m *db.MCPServer) { m.Args = json.RawMessage(`{"arg":"bad"}`) }},
		{"malformed environment", func(m *db.MCPServer) { m.Env = json.RawMessage(`{"MODE":1}`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := m
			tc.change(&changed)
			if validateMCPLaunch(&changed) == nil {
				t.Fatal("modified recipe allowed")
			}
		})
	}
	for _, policy := range []string{"invalid", "null", "[]", `[{"command":"sh"}]`} {
		t.Setenv("ARTEX_MCP_STDIO_ALLOWLIST", policy)
		if validateMCPLaunch(&m) == nil {
			t.Fatalf("policy %q allowed launch", policy)
		}
	}
	if err := validateMCPLaunch(&db.MCPServer{Transport: "http"}); err != nil {
		t.Fatal(err)
	}
}

func TestMCPConnectionDeniesStoredLaunchBeforeSpawn(t *testing.T) {
	t.Setenv("ARTEX_MCP_STDIO_ALLOWLIST", "")
	marker := filepath.Join(t.TempDir(), "spawned")
	command := "/bin/sh"
	args := []string{"-c", "echo spawned > " + marker}
	if os.PathSeparator == '\\' {
		command = "cmd.exe"
		args = []string{"/c", "echo spawned > " + marker}
	}
	raw, _ := json.Marshal(args)
	_, err := connectMCP(context.Background(), &db.MCPServer{Transport: "stdio", Command: command, Args: raw})
	if err == nil {
		t.Fatal("unapproved stored server connected")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("unapproved command executed")
	}
}
