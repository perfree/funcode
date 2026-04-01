package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfree/funcode/internal/logger"
	"github.com/perfree/funcode/pkg/types"
)

// HintEngine generates contextual hints injected between tool results and the
// next LLM call in the agent loop. It analyzes tool results and loop state to
// produce hints that steer the agent toward better behavior.
type HintEngine struct {
	consecutiveErrors int
	iteration         int
	totalErrors       int
	actionRetried     bool            // prevents infinite re-prompting for missed actions
	fileModTools      map[string]bool // "Write", "Edit", "Patch"
}

// LoopState captures the current state of the agent loop, passed to
// GenerateHint so it can decide which hint (if any) to produce.
type LoopState struct {
	Iteration int
	ToolCalls []types.ToolCall
	Results   []types.ToolResult
	MaxIter   int
}

// NewHintEngine creates a HintEngine with default file-modification tool names.
func NewHintEngine() *HintEngine {
	return &HintEngine{
		fileModTools: map[string]bool{
			"Write": true,
			"Edit":  true,
			"Patch": true,
		},
	}
}

// GenerateHint returns a single hint string based on the current loop state.
// Hints are evaluated in priority order; the first applicable hint wins.
// Returns "" when no hint is applicable.
func (h *HintEngine) GenerateHint(state LoopState) string {
	h.iteration = state.Iteration

	// Track error counts from results.
	hasError := false
	for _, r := range state.Results {
		if r.Error != "" {
			hasError = true
			h.consecutiveErrors++
			h.totalErrors++
		}
	}
	if !hasError {
		h.consecutiveErrors = 0
	}

	// P1: Reflection hint — errors detected.
	if hasError {
		hint := h.reflectionHint(state)
		logger.Debug("hint engine produced reflection hint",
			"iteration", state.Iteration,
			"consecutive_errors", h.consecutiveErrors,
			"total_errors", h.totalErrors,
		)
		return hint
	}

	// P2: Verification hint — a file-modifying tool succeeded.
	if hint := h.verificationHint(state); hint != "" {
		logger.Debug("hint engine produced verification hint",
			"iteration", state.Iteration,
		)
		return hint
	}

	// P3: Progress check — every 4 iterations.
	if hint := h.progressHint(state); hint != "" {
		logger.Debug("hint engine produced progress hint",
			"iteration", state.Iteration,
		)
		return hint
	}

	// P4: Urgency hint — approaching max iterations.
	if hint := h.urgencyHint(state); hint != "" {
		logger.Debug("hint engine produced urgency hint",
			"iteration", state.Iteration,
			"max_iter", state.MaxIter,
		)
		return hint
	}

	return ""
}

// reflectionHint asks the agent to analyze root causes after tool errors. If
// errors have been consecutive for 3+ rounds it pushes the agent to try a
// completely different approach.
func (h *HintEngine) reflectionHint(state LoopState) string {
	// Collect the distinct error messages for context.
	var errMsgs []string
	for _, r := range state.Results {
		if r.Error != "" {
			msg := r.Error
			if len(msg) > 200 {
				msg = msg[:200] + "..."
			}
			errMsgs = append(errMsgs, msg)
		}
	}

	var b strings.Builder
	b.WriteString("[SYSTEM HINT] One or more tool calls returned errors. ")
	b.WriteString("Before retrying, analyze the root cause of each error:\n")
	for i, m := range errMsgs {
		b.WriteString(fmt.Sprintf("  Error %d: %s\n", i+1, m))
	}

	if h.consecutiveErrors >= 3 {
		b.WriteString(fmt.Sprintf(
			"WARNING: %d consecutive errors detected. The current approach is likely wrong. "+
				"Stop repeating the same actions and try a completely different strategy.",
			h.consecutiveErrors,
		))
	}

	return b.String()
}

// verificationHint reminds the agent to read modified files and run tests after
// a successful file-modification tool call.
func (h *HintEngine) verificationHint(state LoopState) string {
	if !h.hasFileModification(state) {
		return ""
	}

	// Extract file paths from the successful file-modification calls.
	var paths []string
	for i, tc := range state.ToolCalls {
		if !h.fileModTools[tc.Name] {
			continue
		}
		// Skip if the corresponding result was an error.
		if i < len(state.Results) && state.Results[i].Error != "" {
			continue
		}
		if p := extractFilePath(tc.Params); p != "" {
			paths = append(paths, p)
		}
	}

	if len(paths) == 0 {
		return "[SYSTEM HINT] A file was modified successfully. Remember to read the modified file to verify correctness and run any relevant tests."
	}

	var b strings.Builder
	b.WriteString("[SYSTEM HINT] The following files were modified successfully:\n")
	for _, p := range paths {
		b.WriteString(fmt.Sprintf("  - %s\n", p))
	}
	b.WriteString("Remember to read the modified files to verify correctness and run any relevant tests.")
	return b.String()
}

// progressHint fires every 4 iterations to prompt the agent to review its
// overall goal, progress, and remaining work.
func (h *HintEngine) progressHint(state LoopState) string {
	if state.Iteration > 0 && state.Iteration%4 == 0 {
		return fmt.Sprintf(
			"[SYSTEM HINT] Progress check (iteration %d): Review your original goal, "+
				"assess what has been accomplished so far, and identify what still remains. "+
				"Adjust your plan if needed before continuing.",
			state.Iteration,
		)
	}
	return ""
}

// urgencyHint warns the agent when it is close to the maximum iteration limit.
func (h *HintEngine) urgencyHint(state LoopState) string {
	if state.MaxIter > 0 && state.Iteration >= state.MaxIter-3 {
		remaining := state.MaxIter - state.Iteration
		return fmt.Sprintf(
			"[SYSTEM HINT] URGENT: Only %d iterations remaining before the maximum limit (%d). "+
				"Wrap up your current task now. Prioritize delivering a working result over perfection.",
			remaining, state.MaxIter,
		)
	}
	return ""
}

// hasFileModification returns true when at least one tool call in the state is
// a file-modification tool that succeeded (no error in the corresponding result).
func (h *HintEngine) hasFileModification(state LoopState) bool {
	for i, tc := range state.ToolCalls {
		if h.fileModTools[tc.Name] {
			if i < len(state.Results) && state.Results[i].Error == "" {
				return true
			}
		}
	}
	return false
}

// DetectMissedAction checks if the agent's text response describes actions
// (tool usage, delegation, exploration) without actually producing tool calls.
// Returns a corrective hint, or "" if no issue is detected.
// Only triggers once per agent run to avoid infinite loops.
func (h *HintEngine) DetectMissedAction(response string, teamRoles []TeamRole) string {
	if h.actionRetried {
		return ""
	}

	lower := strings.ToLower(response)

	// --- Check 1: Missed delegation ---
	if len(teamRoles) > 0 {
		delegationPhrases := []string{
			"delegate", "delegating",
			"让", "安排", "交给", "委派", "分配给",
			"i will have", "i'll have", "i'll ask",
			"i will ask", "i'll get", "i will get",
			"让他", "让她", "让它",
			"我会让", "我会安排", "我会要求",
			"我这就安排", "我来安排",
		}
		for _, phrase := range delegationPhrases {
			if strings.Contains(lower, phrase) {
				for _, role := range teamRoles {
					if strings.Contains(lower, strings.ToLower(role.Name)) {
						h.actionRetried = true
						logger.Debug("hint engine detected missed delegation", "mentioned_role", role.Name)
						return fmt.Sprintf(
							"[SYSTEM HINT] You described delegating a task to the '%s' role, "+
								"but you did NOT call the Delegate or Collaborate tool. "+
								"Do not describe delegation — call the Delegate tool now with the task you described.",
							role.Name,
						)
					}
				}
			}
		}
	}

	// --- Check 2: Described tool actions without calling tools ---
	// Detect phrases like "I'll read the files", "let me explore", "我先看看代码"
	actionPhrases := []string{
		// English intent phrases
		"i will read", "i'll read", "let me read",
		"i will look", "i'll look", "let me look",
		"i will check", "i'll check", "let me check",
		"i will explore", "i'll explore", "let me explore",
		"i will analyze", "i'll analyze", "let me analyze",
		"i will review", "i'll review", "let me review",
		"i will search", "i'll search", "let me search",
		"i will examine", "i'll examine", "let me examine",
		"i will inspect", "i'll inspect", "let me inspect",
		"i will trace", "i'll trace", "let me trace",
		"i will scan", "i'll scan", "let me scan",
		"first, let me", "let me start by",
		// Chinese intent phrases
		"我先", "让我先", "我来先",
		"我会先", "我将先",
		"先快速", "先梳理", "先查看", "先读取", "先分析", "先看看", "先了解",
		"我来看", "我来读", "我来查", "我来分析", "我来梳理", "我来检查",
		"接下来我", "然后我会",
	}

	for _, phrase := range actionPhrases {
		if strings.Contains(lower, phrase) {
			h.actionRetried = true
			logger.Debug("hint engine detected described-but-not-executed action", "phrase", phrase)
			return "[SYSTEM HINT] You described what you plan to do, but you did NOT call any tools. " +
				"Do not narrate your intentions — act immediately. " +
				"Call the appropriate tools (Tree, Read, Glob, Grep, etc.) right now to execute the actions you described."
		}
	}

	return ""
}

// extractFilePath attempts to pull a "file_path" value from a JSON params string.
func extractFilePath(paramsJSON string) string {
	if paramsJSON == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &m); err != nil {
		return ""
	}
	if fp, ok := m["file_path"]; ok {
		if s, ok := fp.(string); ok {
			return s
		}
	}
	return ""
}
