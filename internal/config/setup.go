package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultDeveloperPrompt = `You are a senior software developer. You write clean, efficient, and well-tested code.
You have access to tools for reading, writing, and editing files, running shell commands, and searching codebases.
When given a task:
1. First understand the existing codebase by reading relevant files
2. Plan your approach
3. Implement the changes
4. Verify your work
Be concise in your responses. Focus on the code.`

const defaultArchitectPrompt = `You are a senior software architect. You design scalable, maintainable systems.
Focus on:
- System design and architecture decisions
- Design patterns and best practices
- API design and data modeling
- Performance and scalability considerations
- Security architecture
Use read-only tools to analyze codebases. Provide clear, structured architectural guidance.`

// RunSetup runs the interactive first-time setup wizard
func RunSetup() (*Config, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ╭──────────────────────────────────────╮")
	fmt.Println("  │    Welcome to FunCode! 🚀             │")
	fmt.Println("  │     Let's set up your first model.   │")
	fmt.Println("  ╰──────────────────────────────────────╯")
	fmt.Println()

	// 1. Provider selection
	fmt.Println("  Supported providers:")
	fmt.Println("    1) openai")
	fmt.Println("    2) openai-compatible  (DeepSeek, Groq, SiliconFlow, etc.)")
	fmt.Println("    3) anthropic")
	fmt.Println("    4) google")
	fmt.Println("    5) ollama")
	fmt.Println()
	providerChoice := promptInput(reader, "  Select provider [1-5]", "2")
	providerMap := map[string]string{
		"1": "openai", "2": "openai-compatible", "3": "anthropic",
		"4": "google", "5": "ollama",
		"openai": "openai", "openai-compatible": "openai-compatible",
		"anthropic": "anthropic", "google": "google", "ollama": "ollama",
	}
	providerName, ok := providerMap[providerChoice]
	if !ok {
		providerName = "openai-compatible"
	}

	// 2. Model info
	fmt.Println()
	modelName := promptInput(reader, "  Model name (for display, e.g. gpt4o / deepseek)", "default")
	modelID := promptInput(reader, "  Model ID (API model name, e.g. gpt-4o / deepseek-chat)", modelName)

	// 3. API Key
	apiKey := ""
	if providerName != "ollama" {
		apiKey = promptInput(reader, "  API Key", "")
		if apiKey == "" {
			fmt.Println("  ⚠ Warning: API Key is empty. You can set it later in the config file.")
		}
	}

	// 4. Base URL
	baseURL := ""
	if providerName == "openai" {
		baseURL = promptInput(reader, "  Base URL (optional, for proxy/gateway)", "")
	} else if providerName == "openai-compatible" {
		baseURL = promptInput(reader, "  Base URL (e.g. https://api.deepseek.com/v1)", "")
	} else if providerName == "ollama" {
		baseURL = promptInput(reader, "  Ollama URL", "http://localhost:11434")
	}

	// 5. Build config
	model := ModelConfig{
		Name:      modelName,
		Provider:  providerName,
		Model:     modelID,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		MaxTokens: 8192,
	}

	cfg := &Config{
		Models: []ModelConfig{model},
		Roles: []RoleConfig{
			{
				Name:        "developer",
				Description: "高级开发工程师，处理编码任务",
				Model:       modelName,
				Tools:       []string{"*"},
				PromptFile:  "prompts/developer.md",
				Prompt:      defaultDeveloperPrompt,
			},
			{
				Name:        "architect",
				Description: "软件架构师，设计系统和评审架构",
				Model:       modelName,
				Tools:       []string{"Tree", "Read", "ReadRange", "Glob", "Grep", "Diff"},
				PromptFile:  "prompts/architect.md",
				Prompt:      defaultArchitectPrompt,
			},
		},
		DefaultRole: "developer",
		Settings: Settings{
			Theme:            "dark",
			MarkdownRender:   true,
			AutoCompact:      true,
			CompactThreshold: 50000,
			PlanAutoApprove:  false,
		},
		Permissions: []PermissionRule{
			{Tool: "Read", Action: "allow"},
			{Tool: "Glob", Action: "allow"},
			{Tool: "Grep", Action: "allow"},
			{Tool: "Bash", Action: "ask"},
			{Tool: "Write", Action: "ask"},
			{Tool: "Edit", Action: "ask"},
			{Tool: "Delete", Action: "ask"},
		},
	}

	// 6. Write config file
	if err := writePromptFiles(cfg, GlobalConfigDir()); err != nil {
		return nil, fmt.Errorf("writing prompt files: %w", err)
	}
	if err := WriteConfigFile(cfg); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	configPath := filepath.Join(GlobalConfigDir(), "config.yaml")
	fmt.Println()
	fmt.Println("  ✓ Config saved to: " + configPath)
	fmt.Printf("  ✓ Default roles: %s + %s\n", cfg.Roles[0].Name, cfg.Roles[1].Name)
	fmt.Println("  ✓ Prompt files saved under: " + filepath.Join(GlobalConfigDir(), "prompts"))
	fmt.Println("  ✓ You can edit the config file and prompt files to customize roles.")
	fmt.Println()

	return cfg, nil
}

// WriteConfigFile writes config to ~/.funcode/config.yaml as YAML
func WriteConfigFile(cfg *Config) error {
	configPath := filepath.Join(GlobalConfigDir(), "config.yaml")

	var b strings.Builder

	// Models
	b.WriteString("models:\n")
	for _, m := range cfg.Models {
		b.WriteString(fmt.Sprintf("  - name: %q\n", m.Name))
		b.WriteString(fmt.Sprintf("    provider: %q\n", m.Provider))
		b.WriteString(fmt.Sprintf("    model: %q\n", m.Model))
		if m.APIKey != "" {
			b.WriteString(fmt.Sprintf("    api_key: %q\n", m.APIKey))
		}
		if m.BaseURL != "" {
			b.WriteString(fmt.Sprintf("    base_url: %q\n", m.BaseURL))
		}
		b.WriteString(fmt.Sprintf("    max_tokens: %d\n", m.MaxTokens))
		b.WriteString("\n")
	}

	// Roles
	b.WriteString("roles:\n")
	for _, r := range cfg.Roles {
		b.WriteString(fmt.Sprintf("  - name: %q\n", r.Name))
		if r.Description != "" {
			b.WriteString(fmt.Sprintf("    description: %q\n", r.Description))
		}
		b.WriteString(fmt.Sprintf("    model: %q\n", r.Model))
		// Tools
		if len(r.Tools) > 0 {
			toolStrs := make([]string, len(r.Tools))
			for i, t := range r.Tools {
				toolStrs[i] = fmt.Sprintf("%q", t)
			}
			b.WriteString(fmt.Sprintf("    tools: [%s]\n", strings.Join(toolStrs, ", ")))
		}
		b.WriteString(fmt.Sprintf("    prompt_file: %q\n", r.PromptFile))
		b.WriteString("\n")
	}

	// Default role
	b.WriteString(fmt.Sprintf("default_role: %q\n\n", cfg.DefaultRole))

	// Permissions
	b.WriteString("permissions:\n")
	for _, p := range cfg.Permissions {
		b.WriteString(fmt.Sprintf("  - tool: %q\n", p.Tool))
		b.WriteString(fmt.Sprintf("    action: %q\n", p.Action))
		if p.Pattern != "" {
			b.WriteString(fmt.Sprintf("    pattern: %q\n", p.Pattern))
		}
	}
	b.WriteString("\n")

	// Settings
	b.WriteString("settings:\n")
	b.WriteString(fmt.Sprintf("  theme: %q\n", cfg.Settings.Theme))
	b.WriteString(fmt.Sprintf("  markdown_render: %v\n", cfg.Settings.MarkdownRender))
	b.WriteString(fmt.Sprintf("  auto_compact: %v\n", cfg.Settings.AutoCompact))
	b.WriteString(fmt.Sprintf("  compact_threshold: %d\n", cfg.Settings.CompactThreshold))
	b.WriteString(fmt.Sprintf("  plan_auto_approve: %v\n", cfg.Settings.PlanAutoApprove))

	return os.WriteFile(configPath, []byte(b.String()), 0644)
}

func writePromptFiles(cfg *Config, baseDir string) error {
	for _, role := range cfg.Roles {
		if strings.TrimSpace(role.PromptFile) == "" {
			return fmt.Errorf("role %q is missing prompt_file", role.Name)
		}
		if strings.TrimSpace(role.Prompt) == "" {
			return fmt.Errorf("role %q prompt is empty", role.Name)
		}

		promptPath := role.PromptFile
		if !filepath.IsAbs(promptPath) {
			promptPath = filepath.Join(baseDir, promptPath)
		}
		promptPath = filepath.Clean(promptPath)

		if err := os.MkdirAll(filepath.Dir(promptPath), 0755); err != nil {
			return fmt.Errorf("creating prompt dir for role %q: %w", role.Name, err)
		}
		if err := os.WriteFile(promptPath, []byte(role.Prompt+"\n"), 0644); err != nil {
			return fmt.Errorf("writing prompt file for role %q: %w", role.Name, err)
		}
	}
	return nil
}

func promptInput(reader *bufio.Reader, label string, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}
