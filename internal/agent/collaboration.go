package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/perfree/funcode/internal/logger"
)

type CollaborationTaskStatus string

const (
	CollaborationTaskPending   CollaborationTaskStatus = "pending"
	CollaborationTaskRunning   CollaborationTaskStatus = "running"
	CollaborationTaskCompleted CollaborationTaskStatus = "completed"
	CollaborationTaskFailed    CollaborationTaskStatus = "failed"
)

type CollaborationTask struct {
	ID          string
	Role        string
	Title       string
	Task        string
	DependsOn   []string
	Status      CollaborationTaskStatus
	Result      string
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
}

type CollaborationPlan struct {
	Goal          string
	SharedContext string
	ExtraContext  string
	Tasks         []CollaborationTask
}

type CollaborationResult struct {
	Goal      string
	Tasks     []CollaborationTask
	Summary   string
	Succeeded bool
}

type CollaborationManager struct {
	orchestrator *Orchestrator
}

func NewCollaborationManager(orchestrator *Orchestrator) *CollaborationManager {
	return &CollaborationManager{orchestrator: orchestrator}
}

func (m *CollaborationManager) Execute(ctx context.Context, plan CollaborationPlan) (*CollaborationResult, error) {
	if len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("no collaboration tasks provided")
	}

	logger.Info("collaboration started", "goal", plan.Goal, "tasks", len(plan.Tasks))

	tasks := make([]CollaborationTask, len(plan.Tasks))
	copy(tasks, plan.Tasks)

	completed := make(map[string]bool, len(tasks))
	for len(completed) < len(tasks) {
		var ready []*CollaborationTask
		readyPrompts := make(map[string]string)

		for i := range tasks {
			task := &tasks[i]
			if task.Status == CollaborationTaskCompleted || task.Status == CollaborationTaskFailed {
				continue
			}
			if !dependenciesMet(task.DependsOn, completed) {
				continue
			}
			task.Status = CollaborationTaskRunning
			task.StartedAt = time.Now()
			ready = append(ready, task)
			readyPrompts[task.ID] = buildCollaborationPrompt(plan.Goal, plan.SharedContext, *task, tasks)
		}

		if len(ready) == 0 {
			now := time.Now()
			for i := range tasks {
				task := &tasks[i]
				if task.Status == CollaborationTaskCompleted || task.Status == CollaborationTaskFailed {
					continue
				}
				task.Status = CollaborationTaskFailed
				task.Error = "blocked by unresolved dependency or dependency cycle"
				task.CompletedAt = now
				completed[task.ID] = true
			}
			break
		}

		var wg sync.WaitGroup
		for _, task := range ready {
			wg.Add(1)
			go func(task *CollaborationTask) {
				defer wg.Done()
				logger.Debug("collaboration task start", "task_id", task.ID, "role", task.Role)

				agent := m.orchestrator.GetAgent(task.Role)
				if agent == nil {
					agent = m.orchestrator.GetDefaultAgent()
				}
				if agent == nil {
					task.Status = CollaborationTaskFailed
					task.Error = "no agent available"
					task.CompletedAt = time.Now()
					return
				}

				subAgent := agent.CloneWorker("collab_" + task.ID)
				output, err := subAgent.RunWithContext(ctx, readyPrompts[task.ID], plan.ExtraContext)
				task.CompletedAt = time.Now()
				if err != nil {
					task.Status = CollaborationTaskFailed
					task.Error = err.Error()
					logger.Warn("collaboration task failed", "task_id", task.ID, "error", err)
				} else {
					task.Status = CollaborationTaskCompleted
					task.Result = output
					logger.Debug("collaboration task done", "task_id", task.ID, "output_len", len(output))
				}
			}(task)
		}
		wg.Wait()

		for _, task := range ready {
			completed[task.ID] = true
		}
	}

	summary := aggregateCollaboration(plan.Goal, tasks)
	succeeded := true
	for _, task := range tasks {
		if task.Status != CollaborationTaskCompleted {
			succeeded = false
			break
		}
	}

	logger.Info("collaboration completed", "goal", plan.Goal, "succeeded", succeeded)

	return &CollaborationResult{
		Goal:      plan.Goal,
		Tasks:     tasks,
		Summary:   summary,
		Succeeded: succeeded,
	}, nil
}

func dependenciesMet(dependsOn []string, completed map[string]bool) bool {
	for _, dep := range dependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func buildCollaborationPrompt(goal string, sharedContext string, task CollaborationTask, all []CollaborationTask) string {
	var b strings.Builder
	b.WriteString("You are participating in a multi-role collaboration workflow.\n")
	b.WriteString("You MUST read actual source code and verify facts before drawing conclusions.\n")
	b.WriteString("The provided context is a starting point — always explore the codebase to build genuine understanding.\n")
	b.WriteString("Return concrete, evidence-based results for your assigned task with specific file paths and code references.\n\n")
	b.WriteString("Overall goal:\n")
	b.WriteString(goal)
	if strings.TrimSpace(sharedContext) != "" {
		b.WriteString("\n\nKnown shared context:\n")
		b.WriteString(strings.TrimSpace(sharedContext))
	}
	b.WriteString("\n\nAssigned task:\n")
	if task.Title != "" {
		b.WriteString(task.Title)
		b.WriteString("\n")
	}
	b.WriteString(task.Task)
	b.WriteString("\n\nTask context:\n")
	for _, item := range all {
		if item.ID == task.ID {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item.ID)
		b.WriteString(" [")
		b.WriteString(item.Role)
		b.WriteString("]")
		if item.Title != "" {
			b.WriteString(" ")
			b.WriteString(item.Title)
		}
		if item.Status == CollaborationTaskCompleted && item.Result != "" {
			b.WriteString(" => ")
			b.WriteString(item.Result)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func aggregateCollaboration(goal string, tasks []CollaborationTask) string {
	var b strings.Builder
	b.WriteString("Collaboration goal: ")
	b.WriteString(goal)
	b.WriteString("\n\n")
	for _, task := range tasks {
		b.WriteString("- ")
		b.WriteString(task.ID)
		b.WriteString(" [")
		b.WriteString(task.Role)
		b.WriteString("] ")
		if task.Title != "" {
			b.WriteString(task.Title)
		} else {
			b.WriteString(task.Task)
		}
		b.WriteString(" => ")
		if task.Status == CollaborationTaskCompleted {
			b.WriteString(task.Result)
		} else {
			b.WriteString("ERROR: ")
			b.WriteString(task.Error)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
