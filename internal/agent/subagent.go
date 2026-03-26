package agent

import (
	"context"
	"fmt"

	"github.com/perfree/funcode/internal/logger"
	"golang.org/x/sync/errgroup"
)

// SubTask defines a task for a sub-agent
type SubTask struct {
	ID     string
	Prompt string
	Role   string // which role/agent to use
}

// SubResult holds the result of a sub-agent execution
type SubResult struct {
	TaskID string
	Output string
	Error  error
}

// SubAgentManager manages parallel sub-agent execution
type SubAgentManager struct {
	orchestrator *Orchestrator
	maxParallel  int
}

// NewSubAgentManager creates a new sub-agent manager
func NewSubAgentManager(orchestrator *Orchestrator, maxParallel int) *SubAgentManager {
	if maxParallel <= 0 {
		maxParallel = 10
	}
	return &SubAgentManager{
		orchestrator: orchestrator,
		maxParallel:  maxParallel,
	}
}

// RunParallel executes multiple tasks in parallel using goroutines
func (m *SubAgentManager) RunParallel(ctx context.Context, tasks []SubTask) ([]SubResult, error) {
	logger.Info("sub-agent parallel start", "tasks", len(tasks), "max_parallel", m.maxParallel)
	results := make([]SubResult, len(tasks))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(m.maxParallel)

	for i, task := range tasks {
		i, task := i, task
		g.Go(func() error {
			logger.Debug("sub-agent task start", "task_id", task.ID, "role", task.Role)
			agent := m.orchestrator.GetAgent(task.Role)
			if agent == nil {
				agent = m.orchestrator.GetDefaultAgent()
			}
			if agent == nil {
				logger.Warn("sub-agent no agent available", "task_id", task.ID, "role", task.Role)
				results[i] = SubResult{
					TaskID: task.ID,
					Error:  fmt.Errorf("no agent available for role %q", task.Role),
				}
				return nil
			}

			subAgent := agent.CloneWorker("sub_" + task.ID)
			output, err := subAgent.Run(ctx, task.Prompt)
			if err != nil {
				logger.Warn("sub-agent task failed", "task_id", task.ID, "error", err)
			} else {
				logger.Debug("sub-agent task done", "task_id", task.ID, "output_len", len(output))
			}
			results[i] = SubResult{
				TaskID: task.ID,
				Output: output,
				Error:  err,
			}
			return nil // Don't cancel other tasks on error
		})
	}

	g.Wait()

	succeeded := 0
	for _, r := range results {
		if r.Error == nil {
			succeeded++
		}
	}
	logger.Info("sub-agent parallel done", "total", len(tasks), "succeeded", succeeded, "failed", len(tasks)-succeeded)

	return results, nil
}
