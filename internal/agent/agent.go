package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

	// Inject environment info
	cwd, _ := os.Getwd()
	b.WriteString("\n\n## Environment\n\n")
	b.WriteString(fmt.Sprintf("- OS: %s/%s\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("- Working Directory: %s\n", cwd))
	if runtime.GOOS == "windows" {
		b.WriteString("- Shell: cmd.exe (use Windows commands like `dir`, `type`, NOT `ls`, `cat`)\n")
		b.WriteString("- Path separator: \\ (backslash)\n")
	} else {
		b.WriteString("- Shell: bash\n")
		b.WriteString("- Path separator: / (forward slash)\n")
	}
	b.WriteString("- You can access ANY absolute path on this machine. You are NOT limited to the working directory.\n")
	b.WriteString("- When the user gives you a path like `E:\\other\\test`, you MUST directly use tools (Read, Glob, Write, etc.) on that path. Do NOT say you cannot access it.\n")

	if len(a.Tools) > 0 {
		b.WriteString("\n## Available Tools\n\n")
		b.WriteString("You MUST use the provided tools to complete tasks. ")
		b.WriteString("When the user asks you to create, edit, or read files, run commands, or search code, ")
		b.WriteString("you MUST call the appropriate tool function — do NOT just output the content as text.\n\n")
		b.WriteString("**Important rules:**\n")
		b.WriteString("- To read a whole file or check if it exists → use the **Read** tool (NOT Bash with cat/type)\n")
		b.WriteString("- To read only a specific code section → use the **ReadRange** tool\n")
		b.WriteString("- To inspect project structure quickly → use the **Tree** tool\n")
		b.WriteString("- To list/search files in a directory → use the **Glob** tool (NOT Bash with ls/dir)\n")
		b.WriteString("- To search file contents → use the **Grep** tool (NOT Bash with grep/findstr)\n")
		b.WriteString("- To create a new file → use the **Write** tool\n")
		b.WriteString("- To modify an existing file → use the **Edit** tool (string mode with old_string/new_string, or line mode with line_start/line_end)\n")
		b.WriteString("- To apply a multi-hunk unified diff → use the **Patch** tool\n")
		b.WriteString("- To inspect current git changes → use the **Diff** tool\n")
		b.WriteString("- To run build/test/lint/verify tasks → prefer the **RunTask** tool\n")
		b.WriteString("- To run arbitrary shell commands (build, test, git, etc.) when no better structured tool fits → use the **Bash** tool\n")
		b.WriteString("- To delete a file → use the **Delete** tool (ALWAYS use Glob first to verify the path)\n")
		b.WriteString("- ALWAYS prefer Tree/Read/ReadRange/Glob/Grep/Diff over Bash for inspection tasks — they are faster and safer\n\n")

		// Team delegation guidance
		if len(a.TeamRoles) > 0 {
			b.WriteString("**Team collaboration:**\n")
			b.WriteString("You have a **Delegate** tool to ask other specialist roles for help. ")
			b.WriteString("Decide first whether you should inspect the project yourself or delegate immediately. ")
			b.WriteString("If the user explicitly wants another role to inspect/analyze/review the project, and does NOT ask you to evaluate it first, ")
			b.WriteString("delegate directly with a concise task instead of reading files yourself. ")
			b.WriteString("Only gather context first when your own analysis is required before delegation, or when you already know a small amount of high-value context that will materially help the delegate.\n\n")
			b.WriteString("Pass concise context only when it is truly useful. Do NOT paste long tree listings, large file summaries, or redundant observations if the delegate can inspect the repo directly.\n\n")
			b.WriteString("Preferred workflow for explicit role requests: Brief acknowledgement → Delegate directly → Wait for delegate result → Summarize the delegate's findings for the user.\n\n")
			b.WriteString("Your team members:\n")
			for _, r := range a.TeamRoles {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", r.Name, r.Description))
			}
			b.WriteString("\nYou decide autonomously when to delegate. Don't ask the user for permission — just delegate when it makes sense.\n\n")
		}

		// Parallel sub-agent guidance
		b.WriteString("**Parallel execution with Agent tool:**\n")
		b.WriteString("You have an **Agent** tool that spawns a worker sub-agent to handle a task independently. ")
		b.WriteString("**When you call multiple Agent tools in the SAME response, they run in parallel automatically.**\n")
		b.WriteString("**If the user asks for parallel work across independent items, you MUST emit all needed Agent calls in one response instead of waiting for one result before launching the next.**\n")
		b.WriteString("Worker sub-agents do not recursively orchestrate more Agent/Delegate/Collaborate calls.\n\n")
		b.WriteString("**You decide autonomously whether to use Agent.** Consider using it when:\n")
		b.WriteString("- There are many independent files to create or modify, and parallelism would speed things up\n")
		b.WriteString("- Sub-tasks are complex enough to benefit from isolated context\n")
		b.WriteString("- The user explicitly requests parallel or concurrent execution\n\n")
		b.WriteString("**Prefer NOT to use Agent when:**\n")
		b.WriteString("- Only 1-2 simple files — direct Write/Edit is faster and simpler\n")
		b.WriteString("- Tasks have dependencies — do them sequentially yourself\n")
		b.WriteString("- The task is straightforward enough to handle directly\n\n")
		b.WriteString("**Workflow when you choose to use Agent:**\n")
		b.WriteString("1. **Gather context first**: Read project structure, reference files, understand patterns.\n")
		b.WriteString("2. **Plan the work**: Decide the complete code for every file.\n")
		b.WriteString("3. **Call multiple Agents in ONE response** for true parallelism:\n")
		b.WriteString("   - Include complete context in each Agent task — sub-agents have no prior knowledge.\n")
		b.WriteString("   - Group related files per agent (2-5 files each).\n\n")

		b.WriteString("Here are all the tools:\n\n")

		for _, t := range a.Tools {
			b.WriteString(fmt.Sprintf("### %s\n%s\n\n", t.Name(), t.Description()))
		}
	}

	return b.String()
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

	for iterations := 0; iterations < 50; iterations++ { // safety limit
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

		// If no tool calls, we're done
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
			logger.Info("agent run completed", "agent_id", a.ID, "iterations", iterations+1, "response_len", len(response))
			return response, nil
		}

		emptyResponseRecoveries = 0

		// Execute tool calls (parallel for non-approval, serial for approval-required)
		results := a.executeToolCallsParallel(ctx, toolCalls)

		// Add tool results to memory
		a.Memory.AddToolResults(results)
		messages = a.Memory.Messages()
	}

	logger.Error("agent exceeded max iterations", "agent_id", a.ID)
	return "", fmt.Errorf("agent exceeded maximum iterations")
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
