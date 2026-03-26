package llm

import (
	"fmt"
	"sync"
)

// ProviderConfig holds the configuration to create a provider
type ProviderConfig struct {
	Name        string
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	FixedParams bool // true = don't send temperature/top_p/stop (for models like Codex)
}

// ProviderFactory creates a provider from config
type ProviderFactory func(cfg *ProviderConfig) (Provider, error)

// Registry manages provider factories
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory
}

// NewRegistry creates a new registry
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ProviderFactory),
	}
}

// Register adds a provider factory
func (r *Registry) Register(name string, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Create creates a provider by name
func (r *Registry) Create(providerName string, cfg *ProviderConfig) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.factories[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
	return factory(cfg)
}

// DefaultRegistry is the global provider registry
var DefaultRegistry = NewRegistry()
