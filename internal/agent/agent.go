package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/perfree/funcode/internal/llm"
	"github.com/perfree/funcode/internal/logger"
	"github.com/perfree/funcode/internal/tool"
	"github.com/perfree/funcode/pkg/types"
)

// StreamCallback is called for each stream event (for TUI rendering)
type StreamCallback func(event types.StreamEvent)

// Agent represents an AI agent with a role, LLM provider, and tools
type Agent struct {
	ID               string
	Role             *Role
	Provider         llm.Provider
	Tools            []tool.Tool
	ToolExecutor     *tool.Executor
	Memory           *Memory
	BaseSystemPrompt string
	SystemPrompt     string

	// TeamRoles lists other available roles this agent can delegate to
	TeamRoles []TeamRole

	// Callbacks
	OnStream     StreamCallback
	OnToolCall   func(call types.ToolCall)
	OnToolResult func(call types.ToolCall, result types.ToolResult)

	hintEngine *HintEngine

	runtimeMu          sync.RWMutex
	activeExtraContext string
}

// TeamRole describes another agent available for delegation
type TeamRole struct {
	Name        string
	Description string
}

// AgentConfig holds configuration for creating an agent
type AgentConfig struct {
	ID           string
	Role         *Role
	Provider     llm.Provider
	Tools        []tool.Tool
	ToolExecutor *tool.Executor
	SystemPrompt string
	MaxMemory    int
	TeamRoles    []TeamRole
}

// NewAgent creates a new agent
func NewAgent(cfg AgentConfig) *Agent {
	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" && cfg.Role != nil {
		systemPrompt = cfg.Role.Config.Prompt
	}

	a := &Agent{
		ID:               cfg.ID,
		Role:             cfg.Role,
		Provider:         cfg.Provider,
		Tools:            cfg.Tools,
		ToolExecutor:     cfg.ToolExecutor,
		Memory:           NewMemory(cfg.MaxMemory),
		BaseSystemPrompt: systemPrompt,
		SystemPrompt:     systemPrompt,
		TeamRoles:        cfg.TeamRoles,
		hintEngine:       NewHintEngine(),
	}

	// Inject tool usage instructions into the system prompt
	a.RefreshSystemPrompt()
	return a
}

func (a *Agent) RefreshSystemPrompt() {
	a.SystemPrompt = a.buildFullSystemPrompt()
}

// buildFullSystemPrompt appends environment info and tool descriptions to the base system prompt
func (a *Agent) buildFullSystemPrompt() string {
	var b strings.Builder
	b.WriteString(a.BaseSystemPrompt)

	// --- Environment ---
	cwd, _ := os.Getwd()
	b.WriteString("\n\n## Environment\n\n")
	b.WriteString(fmt.Sprintf("- OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("- Working Directory: %s\n", cwd))
	if runtime.GOOS == "windows" {
		b.WriteString("- Shell: cmd.exe\n")
		b.WriteString("- Path separator: \\ (backslash)\n")
	} else {
		b.WriteString("- Shell: bash\n")
		b.WriteString("- Path separator: / (forward slash)\n")
	}
	b.WriteString("- You can access ANY absolute path on this machine.\n")

	// --- Project instructions ---
	if instructions := loadProjectInstructions(cwd); instructions != "" {
		b.WriteString("\n## Project Instructions\n\n")
		b.WriteString(instructions)
		b.WriteString("\n")
	}

	// --- Core behaviors ---
	b.WriteString(`
## How to Work

### Exploration — understand before you act
A directory tree alone is NEVER enough to understand a project. You MUST read actual source code.

**Required exploration depth:**
1. Tree to see the layout — this is only step ONE, not the answer.
2. Read the entry point (main.go, index.ts, etc.) and key configuration files.
3. Read 3-5 core source files to understand the real architecture, patterns, and code quality.
4. Use Grep to trace how key functions are called, how modules connect, and how data flows.
5. Only AFTER reading real code should you provide analysis or make changes.

**When the user asks you to "look at", "review", "analyze", or "understand" a project or codebase:**
- You MUST read actual source files, not just list the directory structure.
- Read at minimum: the entry point, the most important module, and any file the user specifically mentioned.
- Base your analysis on what the code actually does, not what the file names suggest.

**Tracing and following code:**
- When you see a function call, find its definition. When you see a type, find where it's declared.
- Use Grep to follow import chains and call graphs across files.
- If a search returns too many results, narrow with glob filters. If too few, broaden the search.
- Keep exploring until you have genuine understanding, not surface-level guesses.

**Before editing:** You MUST Read the target file first. Never edit a file you haven't read in this conversation.

### Doing tasks — act, don't narrate
- ALWAYS call tools directly in your response. NEVER reply with just a description of what you plan to do. If you want to read a file — call Read now. If you want to explore — call Tree and Read now.
- Use the provided tools to act — do NOT just output code as text when the user wants real changes.
- Prefer dedicated tools over Bash: Read over cat, Glob over ls/find, Grep over grep/findstr.
- Only use Bash for system commands (build, test, git, install, etc.) that have no dedicated tool.
- Prefer editing existing files over creating new ones. Don't create unnecessary files.
- Match the existing code style. Don't add features, refactoring, comments, or type annotations beyond what was asked.
- Be careful with destructive operations (delete, overwrite, force-push). Verify paths with Glob/Read first.
- When an error occurs, read the error message and diagnose the root cause. Don't blindly retry the same action.

### Delegation — act, don't describe
- When you decide to delegate work to another role, you MUST call the Delegate tool immediately in the SAME response. NEVER just describe what you would delegate — execute it.
- When the user asks you to have another role do something (e.g., "let the architect review this"), call the Delegate tool right away. Do not reply with a plan of what you will delegate — just do it.
- If a task needs multiple roles, use the Collaborate tool to coordinate them in parallel.
- CRITICAL: When you have outlined a task plan that assigns work to specific roles (e.g., "architect does X, backend does Y, frontend does Z") and the user tells you to start or proceed, you MUST use the Collaborate tool to dispatch ALL tasks to the assigned roles. Do NOT start implementing the tasks yourself — your job as a coordinator is to orchestrate through your team, not to do their work.

### Output style
- Be concise and direct. Lead with the answer or action, not the reasoning.
- For complex tasks, give brief progress updates at natural milestones.
- Show errors and blockers immediately — don't hide them.
`)

	// --- Tools ---
	if len(a.Tools) > 0 {
		b.WriteString("## Tools\n\n")

		// Team delegation
		if len(a.TeamRoles) > 0 {
			b.WriteString("**Team:** You can Delegate to specialist roles. Delegate directly when another role is more appropriate — don't ask permission. Available:\n")
			for _, r := range a.TeamRoles {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", r.Name, r.Description))
			}
			b.WriteString("\n")
		}

		b.WriteString("**Parallel execution:** Multiple Agent tool calls in the same response run in parallel. Use this for independent tasks.\n\n")

		for _, t := range a.Tools {
			b.WriteString(fmt.Sprintf("### %s\n%s\n\n", t.Name(), t.Description()))
		}
	}

	return b.String()
}

// loadProjectInstructions searches for project-level instruction files and returns their combined content.
func loadProjectInstructions(cwd string) string {
	candidates := []string{
		filepath.Join(cwd, "FUNCODE.md"),
		filepath.Join(cwd, ".funcode", "AGENTS.md"),
		filepath.Join(cwd, ".funcode", "instructions.md"),
	}

	// Also check global instructions
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".funcode", "AGENTS.md"),
		)
	}

	var parts []string
	seen := make(map[string]bool)

	for _, path := range candidates {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if seen[absPath] {
			continue
		}
		seen[absPath] = true

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			parts = append(parts, content)
		}
	}

	return strings.Join(parts, "\n\n")
}

func (a *Agent) CloneIsolated(id string) *Agent {
	return &Agent{
		ID:               id,
		Role:             a.Role,
		Provider:         a.Provider,
		Tools:            a.Tools,
		ToolExecutor:     a.ToolExecutor,
		Memory:           NewMemory(100),
		BaseSystemPrompt: a.BaseSystemPrompt,
		SystemPrompt:     a.SystemPrompt,
		TeamRoles:        a.TeamRoles,
		hintEngine:       NewHintEngine(),
	}
}

func (a *Agent) CloneWorker(id string) *Agent {
	clone := a.CloneIsolated(id)
	clone.Tools = FilterWorkerTools(clone.Tools)
	clone.RefreshSystemPrompt()
	return clone
}

func FilterWorkerTools(tools []tool.Tool) []tool.Tool {
	filtered := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		switch t.Name() {
		case "Agent", "Delegate", "Collaborate":
			continue
		default:
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func (a *Agent) CurrentExtraContext() string {
	a.runtimeMu.RLock()
	defer a.runtimeMu.RUnlock()
	return a.activeExtraContext
}

func (a *Agent) setActiveExtraContext(extraContext string) func() {
	a.runtimeMu.Lock()
	prev := a.activeExtraContext
	a.activeExtraContext = strings.TrimSpace(extraContext)
	a.runtimeMu.Unlock()

	return func() {
		a.runtimeMu.Lock()
		a.activeExtraContext = prev
		a.runtimeMu.Unlock()
	}
}

// Run executes the agent loop: LLM call -> tool execution -> repeat
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	return a.RunWithContext(ctx, input, "")
}

// RunWithContext executes the agent loop with extra transient system context.
func (a *Agent) RunWithContext(ctx context.Context, input string, extraContext string) (string, error) {
	restoreExtraContext := a.setActiveExtraContext(extraContext)
	defer restoreExtraContext()

	inputPreview := input
	if len(inputPreview) > 100 {
		inputPreview = inputPreview[:100] + "..."
	}
	logger.Info("agent run started", "agent_id", a.ID, "input", inputPreview)

	messages := a.Memory.BuildMessages(input)
	const emptyResponseRecoveryPrompt = "Your previous response was empty. You must either provide a textual reply to the user or call an appropriate tool. Continue the task and do not return an empty message."
	emptyResponseRecoveries := 0
	systemPrompt := a.SystemPrompt
	if strings.TrimSpace(extraContext) != "" {
		systemPrompt += "\n\n## Active Skill Context\n\n" + strings.TrimSpace(extraContext)
	}

	const maxIterations = 20
	for iterations := 0; iterations < maxIterations; iterations++ { // safety limit
		if err := a.Memory.Compact(ctx, a.Provider, systemPrompt); err != nil {
			return "", fmt.Errorf("context compaction error: %w", err)
		}
		messages = a.Memory.Messages()

		requestMessages := messages
		if emptyResponseRecoveries > 0 {
			requestMessages = append(append([]types.Message{}, messages...), types.NewTextMessage(types.RoleUser, emptyResponseRecoveryPrompt))
		}

		// Build request
		req := &llm.ChatRequest{
			Messages: requestMessages,
			Tools:    tool.ToToolDefs(a.Tools),
			System:   systemPrompt,
		}

		if a.Role != nil {
			if a.Role.Config.Temperature > 0 {
				req.Temperature = a.Role.Config.Temperature
			}
		}

		logger.Debug("llm request building", "agent_id", a.ID, "messages", len(messages), "tools", len(req.Tools))

		// Call LLM with streaming (with retry)
		const maxRetries = 5
		var response string
		var toolCalls []types.ToolCall
		var lastErr error

		for retry := 0; retry <= maxRetries; retry++ {
			stream, err := a.Provider.ChatStream(ctx, req)
			if err != nil {
				lastErr = err
				if ctx.Err() != nil {
					return "", fmt.Errorf("cancelled: %w", ctx.Err())
				}
				cleanErr := cleanErrorMessage(err.Error())
				logger.Warn("llm stream error", "agent_id", a.ID, "error", cleanErr, "retry", retry)
				// Immediately output the error
				if a.OnStream != nil {
					a.OnStream(types.StreamEvent{
						Type:  types.EventError,
						Error: cleanErr,
					})
				}
				if retry < maxRetries {
					backoff := time.Duration(1<<retry) * time.Second
					// Output retry count
					if a.OnStream != nil {
						a.OnStream(types.StreamEvent{
							Type:  types.EventError,
							Error: fmt.Sprintf("%d/%d retrying in %s...", maxRetries, retry+1, backoff),
						})
					}
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-time.After(backoff):
					}
					continue
				}
				return "", fmt.Errorf("LLM error after %d retries: %s", maxRetries, cleanErr)
			}

			// Process stream and collect response
			response, toolCalls, _, err = a.processStream(stream)
			if err != nil {
				lastErr = err
				if ctx.Err() != nil {
					return "", fmt.Errorf("cancelled: %w", ctx.Err())
				}
				cleanErr := cleanErrorMessage(err.Error())
				logger.Warn("stream processing error", "agent_id", a.ID, "error", cleanErr, "retry", retry)
				if a.OnStream != nil {
					a.OnStream(types.StreamEvent{
						Type:  types.EventError,
						Error: cleanErr,
					})
				}
				if retry < maxRetries {
					backoff := time.Duration(1<<retry) * time.Second
					if a.OnStream != nil {
						a.OnStream(types.StreamEvent{
							Type:  types.EventError,
							Error: fmt.Sprintf("%d/%d retrying in %s...", maxRetries, retry+1, backoff),
						})
					}
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-time.After(backoff):
					}
					continue
				}
				return "", fmt.Errorf("stream error after %d retries: %s", maxRetries, cleanErrorMessage(lastErr.Error()))
			}
			break // success
		}

		logger.Info("llm response received", "agent_id", a.ID, "response_len", len(response), "tool_calls", len(toolCalls))

		// Add assistant message to memory
		if response != "" || len(toolCalls) > 0 {
			assistantMsg := a.buildAssistantMessage(response, toolCalls)
			a.Memory.Add(assistantMsg)
		}

		// If no tool calls, we're done (unless we detect missed delegation)
		if len(toolCalls) == 0 {
			if strings.TrimSpace(response) == "" {
				if emptyResponseRecoveries < 2 {
					emptyResponseRecoveries++
					logger.Warn("model returned empty response, retrying with recovery prompt", "agent_id", a.ID, "attempt", emptyResponseRecoveries)
					continue
				}
				logger.Warn("model returned empty response after streaming retries, falling back to non-streaming chat", "agent_id", a.ID, "recovery_attempts", emptyResponseRecoveries)
				fallbackResponse, fallbackToolCalls, _, fallbackErr := a.fallbackChatOnce(ctx, req)
				if fallbackErr != nil {
					logger.Error("fallback chat failed", "agent_id", a.ID, "error", fallbackErr, "recovery_attempts", emptyResponseRecoveries)
					return "", fmt.Errorf("model returned an empty response after %d recovery attempt(s): %w", emptyResponseRecoveries, fallbackErr)
				}
				response = fallbackResponse
				toolCalls = fallbackToolCalls
				if response != "" || len(toolCalls) > 0 {
					assistantMsg := a.buildAssistantMessage(response, toolCalls)
					a.Memory.Add(assistantMsg)
				}
				if len(toolCalls) == 0 {
					if strings.TrimSpace(response) == "" {
						logger.Error("fallback chat also returned empty response", "agent_id", a.ID, "recovery_attempts", emptyResponseRecoveries)
						return "", fmt.Errorf("model returned an empty response after %d recovery attempt(s)", emptyResponseRecoveries)
					}
					emptyResponseRecoveries = 0
					logger.Info("agent run completed via fallback chat", "agent_id", a.ID, "iterations", iterations+1, "response_len", len(response))
					return response, nil
				}
			}
			emptyResponseRecoveries = 0

			// Check for missed actions: agent described actions but didn't call any tools
			if hint := a.hintEngine.DetectMissedAction(response, a.TeamRoles); hint != "" {
				logger.Info("detected missed action, re-entering loop", "agent_id", a.ID)
				a.Memory.AddHint(hint)
				continue
			}

			logger.Info("agent run completed", "agent_id", a.ID, "iterations", iterations+1, "response_len", len(response))
			return response, nil
		}

		emptyResponseRecoveries = 0

		// Execute tool calls (parallel for non-approval, serial for approval-required)
		results := a.executeToolCallsParallel(ctx, toolCalls)

		// Add tool results to memory
		a.Memory.AddToolResults(results)

		// Generate and inject contextual hint
		if hint := a.hintEngine.GenerateHint(LoopState{
			Iteration: iterations,
			ToolCalls: toolCalls,
			Results:   results,
			MaxIter:   maxIterations,
			TeamRoles: a.TeamRoles,
		}); hint != "" {
			a.Memory.AddHint(hint)
		}

		messages = a.Memory.Messages()
	}

	logger.Error("agent exceeded max iterations", "agent_id", a.ID)
	return "", fmt.Errorf("agent exceeded maximum iterations (%d)", maxIterations)
}

// RunStream is like Run but returns immediately and streams via callback
func (a *Agent) RunStream(ctx context.Context, input string) error {
	_, err := a.Run(ctx, input)
	return err
}

// executeToolCallsParallel runs tool calls in parallel where safe.
// Approval-gated and coordination-heavy tools are executed serially; others run concurrently.
func (a *Agent) executeToolCallsParallel(ctx context.Context, toolCalls []types.ToolCall) []types.ToolResult {
	results := make([]types.ToolResult, len(toolCalls))

	// Split into parallel-safe and serial groups
	type indexedCall struct {
		index int
		call  types.ToolCall
	}
	var parallel, serial []indexedCall
	for i, tc := range toolCalls {
		if a.mustRunSerial(tc) {
			serial = append(serial, indexedCall{i, tc})
		} else {
			parallel = append(parallel, indexedCall{i, tc})
		}
	}

	logger.Debug("tool calls dispatching", "agent_id", a.ID, "total", len(toolCalls), "parallel", len(parallel), "serial", len(serial))

	// Execute parallel-safe tools concurrently
	if len(parallel) > 0 {
		var wg sync.WaitGroup
		for _, ic := range parallel {
			wg.Add(1)
			go func(idx int, call types.ToolCall) {
				defer wg.Done()
				if a.OnToolCall != nil {
					a.OnToolCall(call)
				}
				results[idx] = a.ToolExecutor.Execute(ctx, call)
				if a.OnToolResult != nil {
					a.OnToolResult(call, results[idx])
				}
			}(ic.index, ic.call)
		}
		wg.Wait()
	}

	// Execute approval-required tools serially
	for _, ic := range serial {
		if a.OnToolCall != nil {
			a.OnToolCall(ic.call)
		}
		results[ic.index] = a.ToolExecutor.Execute(ctx, ic.call)
		if a.OnToolResult != nil {
			a.OnToolResult(ic.call, results[ic.index])
		}
	}

	return results
}

// needsApproval checks if a tool call requires user approval
func (a *Agent) needsApproval(call types.ToolCall) bool {
	t, err := a.ToolExecutor.GetTool(call.Name)
	if err != nil {
		return false
	}
	return t.RequiresApproval(json.RawMessage(call.Params))
}

func (a *Agent) mustRunSerial(call types.ToolCall) bool {
	if a.needsApproval(call) {
		return true
	}
	switch call.Name {
	case "Delegate":
		return true
	default:
		return false
	}
}

// processStream reads the stream and collects text + tool calls
func (a *Agent) processStream(stream llm.Stream) (string, []types.ToolCall, types.Usage, error) {
	defer stream.Close()

	var text string
	var toolCalls []types.ToolCall
	var currentToolCall *types.ToolCall
	var usage types.Usage

	for {
		event, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				if event.Usage != nil {
					usage = *event.Usage
				}
				if a.OnStream != nil && event.Type != "" {
					a.OnStream(event)
				}
				if event.Type == types.EventDone && currentToolCall != nil {
					toolCalls = append(toolCalls, *currentToolCall)
					currentToolCall = nil
				}
				break
			}
			return text, toolCalls, usage, err
		}

		// Fire stream callback
		if a.OnStream != nil {
			a.OnStream(event)
		}
		if event.Usage != nil {
			usage = *event.Usage
		}

		switch event.Type {
		case types.EventTextDelta:
			text += event.Content

		case types.EventToolCallStart:
			if event.ToolCall != nil {
				tc := *event.ToolCall
				currentToolCall = &tc
			}

		case types.EventToolCallDelta:
			if currentToolCall != nil && event.ToolCall != nil {
				// Params are accumulated in the stream implementation
				currentToolCall.Params = event.ToolCall.Params
			}

		case types.EventToolCallEnd:
			if currentToolCall != nil {
				toolCalls = append(toolCalls, *currentToolCall)
				currentToolCall = nil
			} else if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}

		case types.EventDone:
			// Final event
			if currentToolCall != nil {
				toolCalls = append(toolCalls, *currentToolCall)
				currentToolCall = nil
			}

		case types.EventError:
			return text, toolCalls, usage, fmt.Errorf("stream error: %s", event.Error)
		}
	}

	// Handle any remaining tool call
	if currentToolCall != nil {
		toolCalls = append(toolCalls, *currentToolCall)
	}

	return text, toolCalls, usage, nil
}

func (a *Agent) fallbackChatOnce(ctx context.Context, req *llm.ChatRequest) (string, []types.ToolCall, types.Usage, error) {
	resp, err := a.Provider.Chat(ctx, req)
	if err != nil {
		return "", nil, types.Usage{}, fmt.Errorf("fallback non-stream chat failed: %w", err)
	}
	if a.OnStream != nil {
		usage := resp.Usage
		a.OnStream(types.StreamEvent{
			Type:  types.EventDone,
			Usage: &usage,
		})
	}
	text := strings.TrimSpace(resp.Message.GetText())
	toolCalls := resp.Message.GetToolCalls()
	return text, toolCalls, resp.Usage, nil
}

// buildAssistantMessage creates an assistant message from response text and tool calls
// cleanErrorMessage strips HTML content and extracts a clean error summary.
// API providers sometimes return HTML error pages (e.g., Cloudflare 502/503).
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func cleanErrorMessage(msg string) string {
	// Extract HTTP status code info if present (e.g., "502 Bad Gateway")
	statusHint := ""
	for _, code := range []string{"400", "401", "403", "404", "429", "500", "502", "503", "504"} {
		if strings.Contains(msg, code) {
			statusHint = code
			break
		}
	}

	// If message contains HTML, strip it and extract meaningful text
	if strings.Contains(msg, "<html") || strings.Contains(msg, "<!DOCTYPE") || strings.Contains(msg, "<body") {
		// Try to extract <title> content
		titleStart := strings.Index(msg, "<title>")
		titleEnd := strings.Index(msg, "</title>")
		if titleStart >= 0 && titleEnd > titleStart {
			title := strings.TrimSpace(msg[titleStart+7 : titleEnd])
			if title != "" {
				return title
			}
		}
		// Fallback: strip all tags and take first meaningful line
		cleaned := htmlTagRegex.ReplaceAllString(msg, " ")
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		if len(cleaned) > 120 {
			cleaned = cleaned[:120] + "..."
		}
		if statusHint != "" && !strings.Contains(cleaned, statusHint) {
			return fmt.Sprintf("HTTP %s: %s", statusHint, cleaned)
		}
		return cleaned
	}

	// No HTML, but still truncate very long messages
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return msg
}

func (a *Agent) buildAssistantMessage(text string, toolCalls []types.ToolCall) types.Message {
	var content []types.ContentBlock

	if text != "" {
		content = append(content, types.ContentBlock{
			Type: types.ContentTypeText,
			Text: text,
		})
	}

	for _, tc := range toolCalls {
		tc := tc
		content = append(content, types.ContentBlock{
			Type:     types.ContentTypeToolCall,
			ToolCall: &tc,
		})
	}

	return types.Message{
		Role:    types.RoleAssistant,
		Content: content,
	}
}
