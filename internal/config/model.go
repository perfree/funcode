package config

// ModelConfig defines a model
type ModelConfig struct {
	Name        string  `yaml:"name" mapstructure:"name"`
	Provider    string  `yaml:"provider" mapstructure:"provider"` // openai, anthropic, google, ollama, openai-compatible
	Model       string  `yaml:"model" mapstructure:"model"`
	APIKey      string  `yaml:"api_key" mapstructure:"api_key"`
	BaseURL     string  `yaml:"base_url,omitempty" mapstructure:"base_url"`
	MaxTokens   int     `yaml:"max_tokens,omitempty" mapstructure:"max_tokens"`
	Temperature float64 `yaml:"temperature,omitempty" mapstructure:"temperature"`
	FixedParams bool    `yaml:"fixed_params,omitempty" mapstructure:"fixed_params"` // true = don't send temperature/top_p (for models like Codex that have fixed params)
}

// GetModelByName finds a model config by name
func (c *Config) GetModelByName(name string) *ModelConfig {
	for i := range c.Models {
		if c.Models[i].Name == name {
			return &c.Models[i]
		}
	}
	return nil
}

// GetDefaultModel returns the first model or nil
func (c *Config) GetDefaultModel() *ModelConfig {
	if len(c.Models) > 0 {
		return &c.Models[0]
	}
	return nil
}
