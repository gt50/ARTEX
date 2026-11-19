package server

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Autumn-27/artex/db"
)

// Approval comes from the operator's process environment, never the database,
// workspace, or an API request. Match the entire launch recipe, not just a binary:
// interpreters, package runners, and environment overrides can all execute code.
func validateMCPLaunch(m *db.MCPServer) error {
	if m.Transport != "stdio" {
		return nil
	}
	const denied = "stdio MCP launch is not approved by the operator"
	var approved []struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if err := json.Unmarshal([]byte(os.Getenv("ARTEX_MCP_STDIO_ALLOWLIST")), &approved); err != nil {
		return fmt.Errorf("%s (ARTEX_MCP_STDIO_ALLOWLIST is absent or invalid)", denied)
	}
	var args []string
	var env map[string]string
	if len(m.Args) > 0 {
		if err := json.Unmarshal(m.Args, &args); err != nil {
			return fmt.Errorf("invalid MCP arguments")
		}
	}
	if len(m.Env) > 0 {
		if err := json.Unmarshal(m.Env, &env); err != nil {
			return fmt.Errorf("invalid MCP environment")
		}
	}
	for _, recipe := range approved {
		if filepath.IsAbs(recipe.Command) && recipe.Command == m.Command && slices.Equal(recipe.Args, args) && maps.Equal(recipe.Env, env) {
			return nil
		}
	}
	return fmt.Errorf("%s", denied)
}
