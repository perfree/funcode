package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// PermissionAction defines what to do
type PermissionAction string

const (
	PermissionAllow PermissionAction = "allow"
	PermissionDeny  PermissionAction = "deny"
	PermissionAsk   PermissionAction = "ask"
)

// PermissionRule defines a permission rule
type PermissionRule struct {
	Tool    string `yaml:"tool"`
	Action  string `yaml:"action"`
	Pattern string `yaml:"pattern,omitempty"`
}

// PermissionManager manages tool permissions
type PermissionManager struct {
	rules []PermissionRule
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager(rules []PermissionRule) *PermissionManager {
	return &PermissionManager{rules: rules}
}

// Check returns the permission action for a tool call
func (pm *PermissionManager) Check(toolName string, params json.RawMessage) PermissionAction {
	// Check rules in order (most specific first)
	for _, rule := range pm.rules {
		if !matchTool(rule.Tool, toolName) {
			continue
		}
		if rule.Pattern != "" {
			if !matchPattern(rule.Pattern, string(params)) {
				continue
			}
		}
		return PermissionAction(rule.Action)
	}
	// Default: ask
	return PermissionAsk
}

func matchTool(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	matched, _ := filepath.Match(pattern, name)
	return matched
}

func matchPattern(pattern, params string) bool {
	// Simple substring match for now
	return strings.Contains(params, strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*"))
}
