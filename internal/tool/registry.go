package tool

import (
	"fmt"
	"sync"
)

// Registry manages tools
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns a tool by name
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t, nil
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Clone creates a copy of the registry with all tools
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clone := NewRegistry()
	for name, t := range r.tools {
		clone.tools[name] = t
	}
	return clone
}

// FilterByNames returns tools matching the given names, "*" means all
func (r *Registry) FilterByNames(names []string) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, n := range names {
		if n == "*" {
			return r.List()
		}
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var result []Tool
	for name, t := range r.tools {
		if nameSet[name] {
			result = append(result, t)
		}
	}
	return result
}
