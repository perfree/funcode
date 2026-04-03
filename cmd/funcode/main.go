package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perfree/funcode/internal/agent"
	"github.com/perfree/funcode/internal/config"
	conversationstorage "github.com/perfree/funcode/internal/conversation/storage"
	"github.com/perfree/funcode/internal/llm"
	_ "github.com/perfree/funcode/internal/llm/providers/anthropic" // register anthropic provider
	_ "github.com/perfree/funcode/internal/llm/providers/google"    // register google provider
	_ "github.com/perfree/funcode/internal/llm/providers/ollama"    // register ollama provider
	_ "github.com/perfree/funcode/internal/llm/providers/openai"    // register openai provider
	"github.com/perfree/funcode/internal/logger"
	"github.com/perfree/funcode/internal/skill"
	"github.com/perfree/funcode/internal/tool"
	"github.com/perfree/funcode/internal/tool/builtin"
	"github.com/perfree/funcode/internal/tui"
	"github.com/perfree/funcode/internal/version"
	"github.com/perfree/funcode/pkg/types"
)

var (
	flagModel  string
	flagRole   string
	flagConfig string
	flagResume bool
	flagSimple bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "funcode",
		Short:   "FunCode - AI Agent CLI Tool",
		Long:    "A powerful AI agent CLI supporting multiple models, roles, skills, and MCP protocol.",
		Version: version.Version,
		RunE:    runApp,
	}

	rootCmd.Flags().StringVarP(&flagModel, "model", "m", "", "Model to use (name from config)")
	rootCmd.Flags().StringVarP(&flagRole, "role", "r", "", "Role to use (e.g., developer, architect)")
	rootCmd.Flags().StringVarP(&flagConfig, "config", "c", "", "Path to config file")
	rootCmd.Flags().BoolVar(&flagResume, "resume", false, "Resume the last conversation")
	rootCmd.Flags().BoolVar(&flagSimple, "simple", false, "Simple mode (no TUI, stdin/stdout only, good for IDE debug)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runApp(cmd *cobra.Command, args []string) error {
	// 1. Ensure config directories exist
	if err := config.EnsureConfigDirs(); err != nil {
		return fmt.Errorf("initializing config dirs: %w", err)
	}

	// 2. Load configuration (or run first-time setup)
	var cfg *config.Config
	var err error

	if flagConfig != "" {
		cfg, err = config.LoadFromPath(flagConfig)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else if !config.ConfigExists() {
		cfg, err = config.RunSetup()
		if err != nil {
			return fmt.Errorf("setup failed: %w", err)
		}
	} else {
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	// Apply CLI flag overrides
	if flagRole != "" {
		cfg.DefaultRole = flagRole
	}

	// Initialize logger
	if err := logger.Init(cfg.Settings.LogLevel, cfg.Settings.LogDir); err != nil {
		return fmt.Errorf("initializing logger: %w", err)
	}
	defer logger.Close()
	logger.Info("funcode starting", "version", version.Version, "default_role", cfg.DefaultRole)

	// 3. Resolve model config
	modelCfg := resolveModel(cfg, flagModel)
	if modelCfg == nil {
		fmt.Println("No model configured. Run the setup again by deleting:")
		fmt.Printf("  %s\n", filepath.Join(config.GlobalConfigDir(), "config.yaml"))
		return nil
	}

	// 4. Create LLM Provider
	provider, err := llm.DefaultRegistry.Create(modelCfg.Provider, &llm.ProviderConfig{
		Name:        modelCfg.Name,
		APIKey:      modelCfg.APIKey,
		BaseURL:     modelCfg.BaseURL,
		Model:       modelCfg.Model,
		MaxTokens:   modelCfg.MaxTokens,
		Temperature: modelCfg.Temperature,
		FixedParams: modelCfg.FixedParams,
	})
	if err != nil {
		return fmt.Errorf("creating LLM provider: %w", err)
	}
	logger.Info("llm provider created", "provider", modelCfg.Provider, "model", modelCfg.Model)

	// 5. Create Tool Registry and register builtin tools
	toolRegistry := tool.NewRegistry()
	builtin.RegisterAll(toolRegistry)

	convManager, err := conversationstorage.NewManager()
	if err != nil {
		return fmt.Errorf("creating conversation manager: %w", err)
	}
	defer convManager.Close()

	// 6. Create shared approval callback
	// pRef will be set before TUI starts — shared pointer pattern
	pRef := &tui.ProgramRef{}

	approvalFn := func(toolName string, params json.RawMessage) (tool.ApprovalAction, error) {
		if flagSimple {
			fmt.Printf("[Tool: %s] params: %s\n", toolName, string(params))
			return tool.ApprovalAllow, nil
		}
		// TUI mode: send approval request and wait for user response
		replyCh := make(chan int, 1)
		if pRef.P != nil {
			pRef.P.Send(tui.ApprovalRequestMsg{
				ToolName: toolName,
				Params:   string(params),
				ReplyCh:  replyCh,
			})
			choice := <-replyCh // block until user responds
			switch choice {
			case 0:
				return tool.ApprovalAllow, nil
			case 1:
				return tool.ApprovalAlwaysAllow, nil
			default:
				return tool.ApprovalDeny, nil
			}
		}
		return tool.ApprovalAllow, nil
	}

	// Shared executor for initial agent creation (will be replaced per-agent later)
	toolExecutor := tool.NewExecutor(toolRegistry, approvalFn)
	// Shared approval state across all agents — "always allow" applies session-wide
	sharedApproval := toolExecutor.SharedState()

	// 7. Create Orchestrator and register agents for each role
	orchestrator := agent.NewOrchestrator()

	displayModel := modelCfg.Model
	if modelCfg.Name != "" {
		displayModel = modelCfg.Name
	}

	projectPath, _ := os.Getwd()
	activeConv, err := convManager.OpenOrCreate(projectPath, displayModel, cfg.DefaultRole, flagResume)
	if err != nil {
		return fmt.Errorf("opening conversation: %w", err)
	}

	skillManager, err := skill.Load(projectPath)
	if err != nil {
		return fmt.Errorf("loading skills: %w", err)
	}
	for _, warning := range skillManager.Warnings() {
		logger.Warn("skill load warning", "warning", warning)
	}

	// Build team role list for delegation awareness
	var teamRoles []agent.TeamRole
	for _, roleCfg := range cfg.Roles {
		teamRoles = append(teamRoles, agent.TeamRole{
			Name:        roleCfg.Name,
			Description: roleCfg.Description,
		})
	}

	for _, roleCfg := range cfg.Roles {
		roleProvider := provider
		if roleCfg.Model != "" && roleCfg.Model != modelCfg.Name {
			roleModelCfg := cfg.GetModelByName(roleCfg.Model)
			if roleModelCfg != nil {
				rp, err := llm.DefaultRegistry.Create(roleModelCfg.Provider, &llm.ProviderConfig{
					Name:        roleModelCfg.Name,
					APIKey:      roleModelCfg.APIKey,
					BaseURL:     roleModelCfg.BaseURL,
					Model:       roleModelCfg.Model,
					MaxTokens:   roleModelCfg.MaxTokens,
					Temperature: roleModelCfg.Temperature,
					FixedParams: roleModelCfg.FixedParams,
				})
				if err == nil {
					roleProvider = rp
				}
			}
		}

		roleTools := toolRegistry.FilterByNames(roleCfg.Tools)
		if len(roleTools) == 0 {
			roleTools = toolRegistry.List()
		}

		// Filter team roles: exclude self
		var otherRoles []agent.TeamRole
		for _, tr := range teamRoles {
			if tr.Name != roleCfg.Name {
				otherRoles = append(otherRoles, tr)
			}
		}

		role := agent.NewRole(roleCfg.Name, roleCfg)
		a := agent.NewAgent(agent.AgentConfig{
			ID:           roleCfg.Name,
			Role:         role,
			Provider:     roleProvider,
			Tools:        roleTools,
			ToolExecutor: toolExecutor,
			TeamRoles:    otherRoles,
		})
		a.Memory.SetCompactThreshold(cfg.Settings.CompactThreshold)
		if activeConv != nil {
			a.Memory.Load(activeConv.GetMessages(), activeConv.Session.Summary)
			a.Memory.SetHooks(
				func(msg types.Message) {
					_ = convManager.Append(activeConv, msg)
				},
				func(summary string) {
					_ = convManager.UpdateSummary(activeConv, summary)
				},
			)
		}

		orchestrator.Register(roleCfg.Name, a)
		logger.Debug("agent registered", "role", roleCfg.Name, "tools", len(roleTools))
	}

	orchestrator.SetDefault(cfg.DefaultRole)

	// 8. Create per-agent registries with agent-specific tools (Delegate, Collaborate, Agent)
	// Each agent gets its own Registry + Executor so agent-specific tools
	// resolve to the correct provider/context.
	var roleInfos []builtin.RoleInfo
	for _, tr := range teamRoles {
		roleInfos = append(roleInfos, builtin.RoleInfo{
			Name:        tr.Name,
			Description: tr.Description,
		})
	}

	for _, name := range orchestrator.ListAgents() {
		a := orchestrator.GetAgent(name)
		if a == nil {
			continue
		}
		agentRef := a

		agentRegistry := tool.NewRegistry()
		for _, existingTool := range agentRef.Tools {
			agentRegistry.Register(existingTool)
		}

		delegateTool := builtin.NewDelegateTool(func(ctx context.Context, roleName string, task string, contextText string) (string, error) {
			logger.Info("delegate sub-agent starting", "parent", agentRef.ID, "target_role", roleName)
			target := orchestrator.GetAgent(roleName)
			if target == nil {
				return "", fmt.Errorf("role '%s' not found", roleName)
			}

			// Resolve display name for TUI spinner
			displayName := roleName
			if target.Role != nil && target.Role.Name != "" {
				displayName = target.Role.Name
			}

			subAgent := target.CloneWorker("delegate_" + roleName)
			extraContext := agentRef.CurrentExtraContext()
			parentCallID := tool.CallIDFromContext(ctx)

			// Forward sub-agent activities to TUI as DelegateActivityMsg
			if pRef.P != nil && parentCallID != "" {
				subAgent.OnStream = func(event types.StreamEvent) {
					pRef.P.Send(tui.DelegateStreamMsg{
						ParentCallID: parentCallID,
						RoleName:     displayName,
						Event:        event,
					})
				}
				subAgent.OnToolCall = func(call types.ToolCall) {
					pRef.P.Send(tui.DelegateActivityMsg{
						ParentCallID: parentCallID,
						RoleName:     displayName,
						Call:         call,
					})
				}
				subAgent.OnToolResult = func(call types.ToolCall, result types.ToolResult) {
					pRef.P.Send(tui.DelegateActivityMsg{
						ParentCallID: parentCallID,
						RoleName:     displayName,
						Call:         call,
						Result:       &result,
					})
				}
			}

			delegatedTask := task
			if strings.TrimSpace(contextText) != "" {
				delegatedTask = "Known context (use as a starting point, but always verify by reading actual source code):\n\n" + contextText + "\n\nDelegated task:\n" + task
			}
			delegatedTask += "\n\n## Delegation Depth Requirements\n" +
				"- You MUST read actual source code before drawing any conclusions. Never base your analysis solely on context summaries or file names.\n" +
				"- Read at least the key files relevant to the task. Use Grep to trace how components connect.\n" +
				"- If the task requires code changes: READ the files, then WRITE/EDIT them. Deliver working code, not just a plan.\n" +
				"- If the task is review/analysis: provide specific findings with file paths and line references.\n" +
				"- Do not just describe what you would do — actually do it using the tools available to you.\n"
			return subAgent.RunWithContext(ctx, delegatedTask, extraContext)
		}, roleInfos)
		if toolAllowed(agentRef.Role.Config.Tools, delegateTool.Name()) {
			agentRegistry.Register(delegateTool)
			agentRef.Tools = append(agentRef.Tools, delegateTool)
		}

		collaborateTool := builtin.NewCollaborateTool(func(ctx context.Context, plan agent.CollaborationPlan) (*agent.CollaborationResult, error) {
			plan.ExtraContext = agentRef.CurrentExtraContext()
			manager := agent.NewCollaborationManager(orchestrator)

			// Pass parent call ID so TUI can associate activity with the Collaborate tool block
			parentCallID := tool.CallIDFromContext(ctx)
			manager.SetParentCallID(parentCallID)

			// Wire up TUI callbacks so collaboration tasks show live activity
			if pRef.P != nil && parentCallID != "" {
				manager.SetCallbacks(&agent.CollaborationCallbacks{
					OnSubAgentStream: func(pid string, rn string, event types.StreamEvent) {
						pRef.P.Send(tui.DelegateStreamMsg{
							ParentCallID: pid,
							RoleName:     rn,
							Event:        event,
						})
					},
					OnSubAgentToolCall: func(pid string, rn string, call types.ToolCall) {
						pRef.P.Send(tui.DelegateActivityMsg{
							ParentCallID: pid,
							RoleName:     rn,
							Call:         call,
						})
					},
					OnSubAgentToolResult: func(pid string, rn string, call types.ToolCall, result types.ToolResult) {
						pRef.P.Send(tui.DelegateActivityMsg{
							ParentCallID: pid,
							RoleName:     rn,
							Call:         call,
							Result:       &result,
						})
					},
				})
			}

			return manager.Execute(ctx, plan)
		}, roleInfos)
		if toolAllowed(agentRef.Role.Config.Tools, collaborateTool.Name()) {
			agentRegistry.Register(collaborateTool)
			agentRef.Tools = append(agentRef.Tools, collaborateTool)
		}

		// Sub-agent tool: inherits current agent's provider and tools
		subAgentTool := builtin.NewSubAgentTool(func(ctx context.Context, task string, description string) (string, error) {
			logger.Info("sub-agent starting", "parent", agentRef.ID)
			subAgent := agentRef.CloneWorker("subagent_" + agentRef.ID)
			parentCallID := tool.CallIDFromContext(ctx)
			workerLabel := strings.TrimSpace(description)
			if workerLabel == "" {
				workerLabel = "worker"
			}
			if pRef.P != nil && parentCallID != "" {
				subAgent.OnStream = func(event types.StreamEvent) {
					pRef.P.Send(tui.AgentStreamMsg{
						ParentCallID: parentCallID,
						WorkerLabel:  workerLabel,
						Event:        event,
					})
				}
				subAgent.OnToolCall = func(call types.ToolCall) {
					pRef.P.Send(tui.AgentActivityMsg{
						ParentCallID: parentCallID,
						WorkerLabel:  workerLabel,
						Call:         call,
					})
				}
				subAgent.OnToolResult = func(call types.ToolCall, result types.ToolResult) {
					pRef.P.Send(tui.AgentActivityMsg{
						ParentCallID: parentCallID,
						WorkerLabel:  workerLabel,
						Call:         call,
						Result:       &result,
					})
				}
			}
			return subAgent.RunWithContext(ctx, task, agentRef.CurrentExtraContext())
		})
		if toolAllowed(agentRef.Role.Config.Tools, subAgentTool.Name()) {
			agentRegistry.Register(subAgentTool)
			agentRef.Tools = append(agentRef.Tools, subAgentTool)
		}

		// Give this agent its own executor with the agent-specific registry, sharing session-wide approval state
		agentExecutor := tool.NewExecutorWithSharedState(agentRegistry, approvalFn, sharedApproval)
		agentRef.ToolExecutor = agentExecutor
		agentRef.RefreshSystemPrompt()
	}

	// 8. Launch Simple mode or TUI mode
	if flagSimple {
		return runSimple(orchestrator, displayModel, skillManager)
	}
	return tui.Run(orchestrator, displayModel, cfg.Settings.MarkdownRender, skillManager, pRef)
}

// runSimple runs a simple stdin/stdout REPL (no TUI, works in any terminal/IDE)
func runSimple(orchestrator *agent.Orchestrator, modelName string, skillManager *skill.Manager) error {
	fmt.Printf("%s | Model: %s | Role: %s\n", version.Full(), modelName, orchestrator.GetDefaultAgent().ID)
	fmt.Println("Type your message. /quit to exit, /clear to reset, @role to switch agent.")
	fmt.Println(strings.Repeat("─", 60))

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var activeSkill *skill.Skill

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			fmt.Println("Bye!")
			return nil
		}
		if input == "/clear" {
			fmt.Println("(conversation cleared)")
			continue
		}

		if cmd, ok := skill.ParseCommand(input); ok {
			switch cmd.Action {
			case skill.CommandShow:
				if cmd.Name == "" {
					fmt.Println(skillManager.BuildListText())
				} else {
					text, err := skillManager.BuildShowText(cmd.Name)
					if err != nil {
						fmt.Printf("Error: %v\n", err)
					} else {
						fmt.Println(text)
					}
				}
			case skill.CommandUse:
				s, exists := skillManager.Get(cmd.Name)
				if !exists {
					fmt.Printf("Error: skill %q not found\n", cmd.Name)
				} else {
					activeSkill = s
					fmt.Printf("Activated skill: %s\n", s.Name)
				}
			case skill.CommandClear:
				activeSkill = nil
				fmt.Println("Cleared active skill.")
			case skill.CommandRun:
				s, exists := skillManager.Get(cmd.Name)
				if !exists {
					fmt.Printf("Error: skill %q not found\n", cmd.Name)
					continue
				}
				targetAgent, cleanTask := orchestrator.Route(cmd.Task)
				if targetAgent == nil {
					fmt.Println("Error: no agent available")
					continue
				}
				targetAgent.OnStream = func(event types.StreamEvent) {
					if event.Type == types.EventTextDelta {
						fmt.Print(event.Content)
					}
				}
				targetAgent.OnToolCall = func(call types.ToolCall) {
					fmt.Printf("\n[Tool: %s] ...\n", call.Name)
				}
				targetAgent.OnToolResult = func(call types.ToolCall, result types.ToolResult) {
					if result.Error != "" {
						fmt.Printf("[Tool: %s] Error: %s\n", call.Name, result.Error)
					} else {
						fmt.Printf("[Tool: %s] Done (%d chars)\n", call.Name, len(result.Content))
					}
				}
				_, err := skill.RunWithSkill(context.Background(), targetAgent, cleanTask, s)
				if err != nil {
					fmt.Printf("\nError: %v\n", err)
				}
				fmt.Println()
			default:
				fmt.Println(skill.HelpText(activeSkill))
			}
			continue
		}

		// Route to agent
		targetAgent, cleanInput := orchestrator.Route(input)
		if targetAgent == nil {
			fmt.Println("Error: no agent available")
			continue
		}

		// Set up streaming callback for real-time output
		targetAgent.OnStream = func(event types.StreamEvent) {
			if event.Type == types.EventTextDelta {
				fmt.Print(event.Content)
			}
		}
		targetAgent.OnToolCall = func(call types.ToolCall) {
			fmt.Printf("\n[Tool: %s] ...\n", call.Name)
		}
		targetAgent.OnToolResult = func(call types.ToolCall, result types.ToolResult) {
			if result.Error != "" {
				fmt.Printf("[Tool: %s] Error: %s\n", call.Name, result.Error)
			} else {
				preview := result.Content
				if len(preview) > 300 {
					preview = preview[:300] + "..."
				}
				fmt.Printf("[Tool: %s] Done (%d chars)\n", call.Name, len(result.Content))
				_ = preview
			}
		}

		// Run agent
		_, err := skill.RunWithSkill(context.Background(), targetAgent, cleanInput, activeSkill)
		if err != nil {
			fmt.Printf("\nError: %v\n", err)
		}
		fmt.Println() // newline after streaming
	}

	return scanner.Err()
}

// resolveModel finds the model config to use
func resolveModel(cfg *config.Config, flagModel string) *config.ModelConfig {
	if flagModel != "" {
		return cfg.GetModelByName(flagModel)
	}

	roleCfg := cfg.GetRole(cfg.DefaultRole)
	if roleCfg != nil && roleCfg.Model != "" {
		m := cfg.GetModelByName(roleCfg.Model)
		if m != nil {
			return m
		}
	}

	return cfg.GetDefaultModel()
}

func toolAllowed(names []string, toolName string) bool {
	if len(names) == 0 {
		return true
	}
	for _, name := range names {
		if name == "*" || name == toolName {
			return true
		}
	}
	return false
}
