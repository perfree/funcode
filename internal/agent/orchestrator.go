package agent

import (
	"strings"
	"sync"
)

// Orchestrator manages multiple agents and routes messages
type Orchestrator struct {
	mu           sync.RWMutex
	agents       map[string]*Agent
	defaultAgent string
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		agents: make(map[string]*Agent),
	}
}

// Register adds an agent
func (o *Orchestrator) Register(name string, agent *Agent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agents[name] = agent
}

// SetDefault sets the default agent
func (o *Orchestrator) SetDefault(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.defaultAgent = name
}

// GetAgent returns an agent by name
func (o *Orchestrator) GetAgent(name string) *Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agents[name]
}

// GetDefaultAgent returns the default agent
func (o *Orchestrator) GetDefaultAgent() *Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agents[o.defaultAgent]
}

// Route determines which agent should handle the message
// Supports @mention syntax: "@architect design the API"
// Returns (nil, msg) for @all — caller should use RouteAll instead
func (o *Orchestrator) Route(input string) (*Agent, string) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	// Check for @mention
	if strings.HasPrefix(input, "@") {
		parts := strings.SplitN(input, " ", 2)
		mention := strings.TrimPrefix(parts[0], "@")
		msg := ""
		if len(parts) > 1 {
			msg = parts[1]
		}

		// @all → return nil to signal broadcast
		if mention == "all" {
			return nil, msg
		}

		if agent, ok := o.agents[mention]; ok {
			return agent, msg
		}
	}

	// Default agent
	agent := o.agents[o.defaultAgent]
	if agent == nil {
		for _, a := range o.agents {
			return a, input
		}
	}
	return agent, input
}

// IsAllMention checks if the input is an @all mention
func (o *Orchestrator) IsAllMention(input string) bool {
	return strings.HasPrefix(input, "@all ") || input == "@all"
}

// RouteAll returns all agents for @all broadcast
func (o *Orchestrator) RouteAll(input string) ([]*Agent, string) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	parts := strings.SplitN(input, " ", 2)
	msg := ""
	if len(parts) > 1 {
		msg = parts[1]
	}

	agents := make([]*Agent, 0, len(o.agents))
	for _, a := range o.agents {
		agents = append(agents, a)
	}
	return agents, msg
}

// ListAgents returns all registered agent names
func (o *Orchestrator) ListAgents() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	names := make([]string, 0, len(o.agents))
	for name := range o.agents {
		names = append(names, name)
	}
	return names
}

// ListAgentRoles returns role display info
func (o *Orchestrator) ListAgentRoles() []AgentInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()
	var infos []AgentInfo
	for name, agent := range o.agents {
		info := AgentInfo{Name: name}
		if agent.Role != nil {
			info.Description = agent.Role.Config.Description
		}
		infos = append(infos, info)
	}
	return infos
}

// AgentInfo holds display info about an agent
type AgentInfo struct {
	Name        string
	Description string
}

func (ai AgentInfo) String() string {
	return ai.Name
}
