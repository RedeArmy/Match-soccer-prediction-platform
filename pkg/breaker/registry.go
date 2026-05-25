package breaker

import "sync"

// Registry tracks active Breakers by name. It is safe for concurrent use.
type Registry struct {
	mu       sync.Mutex
	breakers map[string]*Breaker
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{breakers: make(map[string]*Breaker)}
}

// Register adds b to the registry, replacing any prior entry with the same name.
func (reg *Registry) Register(b *Breaker) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.breakers[b.Name()] = b
}

// All returns a snapshot of all registered breakers in arbitrary order.
func (reg *Registry) All() []*Breaker {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make([]*Breaker, 0, len(reg.breakers))
	for _, b := range reg.breakers {
		out = append(out, b)
	}
	return out
}
