package config

import (
	"os"
	"path/filepath"
)

// Config is the root configuration
type Config struct {
	Models      []ModelConfig     `yaml:"models" mapstructure:"models"`
	Roles       []RoleConfig      `yaml:"roles" mapstructure:"roles"`
	DefaultRole string            `yaml:"default_role" mapstructure:"default_role"`
	MCPServers  []MCPServerConfig `yaml:"mcp_servers" mapstructure:"mcp_servers"`
	Permissions []PermissionRule  `yaml:"permissions" mapstructure:"permissions"`
	Settings    Settings          `yaml:"settings" mapstructure:"settings"`
}

// Settings holds general settings
type Settings struct {
	Theme            string `yaml:"theme" mapstructure:"theme"`
	MarkdownRender   bool   `yaml:"markdown_render" mapstructure:"markdown_render"`
	AutoCompact      bool   `yaml:"auto_compact" mapstructure:"auto_compact"`
	CompactThreshold int    `yaml:"compact_threshold" mapstructure:"compact_threshold"`
	PlanAutoApprove  bool   `yaml:"plan_auto_approve" mapstructure:"plan_auto_approve"`
	LogLevel         string `yaml:"log_level" mapstructure:"log_level"`
	LogDir           string `yaml:"log_dir" mapstructure:"log_dir"`
}

// MCPServerConfig defines an MCP server
type MCPServerConfig struct {
	Name      string            `yaml:"name" mapstructure:"name"`
	Transport string            `yaml:"transport" mapstructure:"transport"` // stdio, sse, streamable-http
	Command   string            `yaml:"command,omitempty" mapstructure:"command"`
	Args      []string          `yaml:"args,omitempty" mapstructure:"args"`
	URL       string            `yaml:"url,omitempty" mapstructure:"url"`
	Env       map[string]string `yaml:"env,omitempty" mapstructure:"env"`
}

// PermissionRule defines a tool permission
type PermissionRule struct {
	Tool    string `yaml:"tool" mapstructure:"tool"`
	Action  string `yaml:"action" mapstructure:"action"` // allow, deny, ask
	Pattern string `yaml:"pattern,omitempty" mapstructure:"pattern"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultRole: "developer",
		Settings: Settings{
			Theme:            "dark",
			MarkdownRender:   true,
			AutoCompact:      true,
			CompactThreshold: 50000,
			PlanAutoApprove:  false,
			LogLevel:         "info",
			LogDir:           "logs",
		},
		Permissions: []PermissionRule{
			{Tool: "Read", Action: "allow"},
			{Tool: "Glob", Action: "allow"},
			{Tool: "Grep", Action: "allow"},
			{Tool: "Bash", Action: "ask"},
			{Tool: "Write", Action: "ask"},
			{Tool: "Edit", Action: "ask"},
		},
	}
}

// GlobalConfigDir returns ~/.funcode/
func GlobalConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".funcode")
}

// ProjectConfigDir returns .funcode/ in the current project
func ProjectConfigDir() string {
	return ".funcode"
}
