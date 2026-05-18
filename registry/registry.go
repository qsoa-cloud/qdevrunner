package registry

import (
	"fmt"
	"math/rand"
	"sync"
)

// Instance represents a running service instance that can be looked up by name.
type Instance interface {
	GetName() string
	GetGrpcToServiceAddr() string
	IsGrpcReady() bool
	GetHttpToServiceAddr() string
	IsHttpReady() bool
}

// Registry maps service discovery names to running instances.
type Registry struct {
	mu        sync.RWMutex
	instances map[string][]Instance // discoveryName -> instances
}

func New() *Registry {
	return &Registry{
		instances: make(map[string][]Instance),
	}
}

// Register adds an instance to the registry.
func (r *Registry) Register(instance Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := instance.GetName()
	r.instances[name] = append(r.instances[name], instance)
}

// Unregister removes an instance from the registry.
func (r *Registry) Unregister(instance Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := instance.GetName()
	list := r.instances[name]
	for i, inst := range list {
		if inst == instance {
			r.instances[name] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(r.instances[name]) == 0 {
		delete(r.instances, name)
	}
}

// GetGrpcInstance returns a random instance with gRPC ready for the given service name.
func (r *Registry) GetGrpcInstance(serviceName string) (Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := r.instances[serviceName]
	if len(list) == 0 {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}

	// Filter to instances with gRPC ready.
	var ready []Instance
	for _, inst := range list {
		if inst.IsGrpcReady() {
			ready = append(ready, inst)
		}
	}
	if len(ready) == 0 {
		return nil, fmt.Errorf("service %q has no ready gRPC instances", serviceName)
	}

	return ready[rand.Intn(len(ready))], nil
}

// GetHttpInstance returns a random instance with HTTP ready for the given service name.
func (r *Registry) GetHttpInstance(serviceName string) (Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := r.instances[serviceName]
	if len(list) == 0 {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}

	var ready []Instance
	for _, inst := range list {
		if inst.IsHttpReady() {
			ready = append(ready, inst)
		}
	}
	if len(ready) == 0 {
		return nil, fmt.Errorf("service %q has no ready HTTP instances", serviceName)
	}

	return ready[rand.Intn(len(ready))], nil
}

// ListAll returns all registered instance names.
func (r *Registry) ListAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.instances))
	for name := range r.instances {
		names = append(names, name)
	}
	return names
}
