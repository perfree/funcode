package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/perfree/funcode/internal/logger"
	"github.com/perfree/funcode/pkg/types"
)

// ApprovalAction represents the user's approval decision
type ApprovalAction int

const (
	ApprovalAllow       ApprovalAction = iota // allow this one time
	ApprovalAlwaysAllow                       // always allow this tool for this session
	ApprovalDeny                              // deny this call
)

// ApprovalFunc is called to ask user for tool approval
// Returns (action, error)
type ApprovalFunc func(toolName string, params json.RawMessage) (ApprovalAction, error)

// SharedApprovalState holds session-wide approval decisions shared across all Executors.
type SharedApprovalState struct {
	mu            sync.RWMutex
	alwaysAllowed map[string]bool
}

// NewSharedApprovalState creates a shared approval state for a session.
func NewSharedApprovalState() *SharedApprovalState {
	return &SharedApprovalState{
		alwaysAllowed: make(map[string]bool),
	}
}

func (s *SharedApprovalState) IsAllowed(toolName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alwaysAllowed[toolName]
}

func (s *SharedApprovalState) SetAllowed(toolName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alwaysAllowed[toolName] = true
}

// Executor executes tool calls
type Executor struct {
	registry    *Registry
	approvalFn  ApprovalFunc
	sharedState *SharedApprovalState
}

// NewExecutor creates a new tool executor
func NewExecutor(registry *Registry, approvalFn ApprovalFunc) *Executor {
	return &Executor{
		registry:    registry,
		approvalFn:  approvalFn,
		sharedState: NewSharedApprovalState(),
	}
}

// NewExecutorWithSharedState creates a new tool executor sharing approval state with others.
func NewExecutorWithSharedState(registry *Registry, approvalFn ApprovalFunc, shared *SharedApprovalState) *Executor {
	return &Executor{
		registry:    registry,
		approvalFn:  approvalFn,
		sharedState: shared,
	}
}

// Execute runs a single tool call and returns a ToolResult
func (e *Executor) Execute(ctx context.Context, call types.ToolCall) types.ToolResult {
	logger.Debug("tool execute start", "tool", call.Name, "call_id", call.ID)

	t, err := e.registry.Get(call.Name)
	if err != nil {
		logger.Warn("tool not found", "tool", call.Name)
		return types.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("tool not found: %s", call.Name),
		}
	}

	params := json.RawMessage(call.Params)

	// Check approval
	if t.RequiresApproval(params) && !e.sharedState.IsAllowed(call.Name) && e.approvalFn != nil {
		logger.Debug("tool requires approval", "tool", call.Name)
		action, err := e.approvalFn(call.Name, params)
		if err != nil {
			logger.Warn("tool approval error", "tool", call.Name, "error", err)
			return types.ToolResult{
				CallID: call.ID,
				Error:  "approval error: " + err.Error(),
			}
		}
		switch action {
		case ApprovalAlwaysAllow:
			logger.Info("tool approved (always)", "tool", call.Name)
			e.sharedState.SetAllowed(call.Name)
			// fall through to execute
		case ApprovalDeny:
			logger.Info("tool denied", "tool", call.Name)
			return types.ToolResult{
				CallID: call.ID,
				Error:  "tool execution denied by user",
			}
		case ApprovalAllow:
			logger.Info("tool approved (once)", "tool", call.Name)
			// one-time allow, fall through
		}
	}

	// Execute
	result, err := t.Execute(WithCallID(ctx, call.ID), params)
	if err != nil {
		logger.Warn("tool execute failed", "tool", call.Name, "error", err)
		return types.ToolResult{
			CallID: call.ID,
			Error:  err.Error(),
		}
	}

	resultLen := len(result.Content)
	if result.Error != "" {
		logger.Warn("tool returned error", "tool", call.Name, "error", result.Error)
	} else {
		logger.Debug("tool execute done", "tool", call.Name, "result_len", resultLen)
	}

	return types.ToolResult{
		CallID:  call.ID,
		Content: result.Content,
		Error:   result.Error,
	}
}

// GetTool returns a tool by name from the registry
func (e *Executor) GetTool(name string) (Tool, error) {
	return e.registry.Get(name)
}

// SharedState returns the shared approval state for creating linked executors.
func (e *Executor) SharedState() *SharedApprovalState {
	return e.sharedState
}

// ExecuteAll runs multiple tool calls sequentially
func (e *Executor) ExecuteAll(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	results := make([]types.ToolResult, len(calls))
	for i, call := range calls {
		results[i] = e.Execute(ctx, call)
	}
	return results
}
