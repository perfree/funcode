package skill

import (
	"context"

	"github.com/perfree/funcode/internal/agent"
)

func RunWithSkill(ctx context.Context, a *agent.Agent, userInput string, active *Skill) (string, error) {
	if active == nil {
		return a.Run(ctx, userInput)
	}
	return a.RunWithContext(ctx, userInput, active.PromptContext())
}
