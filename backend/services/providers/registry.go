package providers

import (
	"fmt"
	"sync"
)

// Registry holds named providers and supports ordered failover lookup.
// The registration order determines failover preference: the first registered
// provider is primary; subsequent entries are tried in order on failure.
type Registry struct {
	mu      sync.RWMutex
	byName  map[string]TranslationProvider
	ordered []TranslationProvider
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]TranslationProvider)}
}

// Register adds p to the registry. If a provider with the same name already
// exists it is replaced in-place, preserving its position in failover order.
func (r *Registry) Register(p TranslationProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[p.Name()]; exists {
		for i, ep := range r.ordered {
			if ep.Name() == p.Name() {
				r.ordered[i] = p
				break
			}
		}
	} else {
		r.ordered = append(r.ordered, p)
	}
	r.byName[p.Name()] = p
}

// Ordered returns a snapshot of all providers in registration (failover) order.
func (r *Registry) Ordered() []TranslationProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TranslationProvider, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// Get returns the named provider or an error if it is not registered.
func (r *Registry) Get(name string) (TranslationProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}

// Len returns the number of registered providers.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ordered)
}
