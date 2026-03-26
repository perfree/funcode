package agent

import "github.com/perfree/funcode/internal/config"

// Role wraps config.RoleConfig with runtime info
type Role struct {
	Name   string
	Config config.RoleConfig
}

// NewRole creates a role from config
func NewRole(name string, cfg config.RoleConfig) *Role {
	return &Role{
		Name:   name,
		Config: cfg,
	}
}
