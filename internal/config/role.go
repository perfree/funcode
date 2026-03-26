package config

// RoleConfig defines a role
type RoleConfig struct {
	Name        string   `yaml:"name" mapstructure:"name"`
	Description string   `yaml:"description,omitempty" mapstructure:"description"`
	Model       string   `yaml:"model" mapstructure:"model"`
	Tools       []string `yaml:"tools,omitempty" mapstructure:"tools"`
	Temperature float64  `yaml:"temperature,omitempty" mapstructure:"temperature"`
	PromptFile  string   `yaml:"prompt_file" mapstructure:"prompt_file"`
	Prompt      string   `yaml:"-" mapstructure:"-"`
}

// GetRole returns the role config by name
func (c *Config) GetRole(name string) *RoleConfig {
	for i := range c.Roles {
		if c.Roles[i].Name == name {
			r := c.Roles[i]
			return &r
		}
	}
	return nil
}
