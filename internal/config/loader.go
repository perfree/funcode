package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Load loads configuration from all sources
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// 1. Load global config
	globalDir := GlobalConfigDir()
	if err := loadFromDir(globalDir, cfg); err != nil {
		_ = err
	}

	// 2. Load project config (overrides global)
	projectDir := ProjectConfigDir()
	if err := loadFromDir(projectDir, cfg); err != nil {
		_ = err
	}

	// 3. Apply environment variable overrides
	applyEnvOverrides(cfg)

	// 4. Resolve environment variable references
	resolveEnvRefs(cfg)

	// 5. Validate loaded configuration
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadFromDir(dir string, cfg *Config) error {
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("reading config %s: %w", configPath, err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("parsing config %s: %w", configPath, err)
	}

	if v.IsSet("roles") {
		if err := loadRolePrompts(cfg, dir); err != nil {
			return err
		}
	}

	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GOCLI_DEFAULT_ROLE"); v != "" {
		cfg.DefaultRole = v
	}
}

func resolveEnvRefs(cfg *Config) {
	for i := range cfg.Models {
		cfg.Models[i].APIKey = expandEnv(cfg.Models[i].APIKey)
		cfg.Models[i].BaseURL = expandEnv(cfg.Models[i].BaseURL)
	}
	for i := range cfg.MCPServers {
		for k, v := range cfg.MCPServers[i].Env {
			cfg.MCPServers[i].Env[k] = expandEnv(v)
		}
	}
}

func expandEnv(s string) string {
	if strings.Contains(s, "${") {
		return os.ExpandEnv(s)
	}
	return s
}

func loadRolePrompts(cfg *Config, baseDir string) error {
	for i := range cfg.Roles {
		role := &cfg.Roles[i]
		if strings.TrimSpace(role.PromptFile) == "" {
			return fmt.Errorf("role %q is missing prompt_file", role.Name)
		}

		promptPath := role.PromptFile
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(baseDir, promptPath)
		}
		promptPath = filepath.Clean(promptPath)

		content, err := os.ReadFile(promptPath)
		if err != nil {
			return fmt.Errorf("reading prompt file for role %q from %s: %w", role.Name, promptPath, err)
		}

		role.Prompt = strings.TrimSpace(string(content))
		if role.Prompt == "" {
			return fmt.Errorf("prompt file for role %q is empty: %s", role.Name, promptPath)
		}
	}
	return nil
}

func validateConfig(cfg *Config) error {
	models := make(map[string]struct{}, len(cfg.Models))
	for _, model := range cfg.Models {
		if strings.TrimSpace(model.Name) == "" {
			return fmt.Errorf("model name cannot be empty")
		}
		models[model.Name] = struct{}{}
	}

	roles := make(map[string]struct{}, len(cfg.Roles))
	for _, role := range cfg.Roles {
		if strings.TrimSpace(role.Name) == "" {
			return fmt.Errorf("role name cannot be empty")
		}
		if _, exists := roles[role.Name]; exists {
			return fmt.Errorf("duplicate role name: %s", role.Name)
		}
		roles[role.Name] = struct{}{}

		if strings.TrimSpace(role.Model) == "" {
			return fmt.Errorf("role %q is missing model", role.Name)
		}
		if _, ok := models[role.Model]; !ok {
			return fmt.Errorf("role %q references unknown model %q", role.Name, role.Model)
		}
		if strings.TrimSpace(role.PromptFile) == "" {
			return fmt.Errorf("role %q is missing prompt_file", role.Name)
		}
		if strings.TrimSpace(role.Prompt) == "" {
			return fmt.Errorf("role %q prompt is empty", role.Name)
		}
	}

	if strings.TrimSpace(cfg.DefaultRole) == "" {
		return fmt.Errorf("default_role cannot be empty")
	}
	if _, ok := roles[cfg.DefaultRole]; !ok {
		return fmt.Errorf("default_role %q is not defined in roles", cfg.DefaultRole)
	}

	return nil
}

// ConfigExists checks if the global config file exists
func ConfigExists() bool {
	configPath := filepath.Join(GlobalConfigDir(), "config.yaml")
	_, err := os.Stat(configPath)
	return err == nil
}

// EnsureConfigDirs creates the necessary config directories
func EnsureConfigDirs() error {
	dirs := []string{
		GlobalConfigDir(),
		filepath.Join(GlobalConfigDir(), "conversations"),
		filepath.Join(GlobalConfigDir(), "memory"),
		filepath.Join(GlobalConfigDir(), "prompts"),
		filepath.Join(GlobalConfigDir(), "skills"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating dir %s: %w", dir, err)
		}
	}
	return nil
}
