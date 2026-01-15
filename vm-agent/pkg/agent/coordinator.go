// Package agent provides the main agent manager.
package agent

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ComponentState represents the state of a component
type ComponentState int

const (
	StateUninitialized ComponentState = iota
	StateInitializing
	StateRunning
	StateStopping
	StateStopped
	StateError
)

// String returns the string representation of a component state
func (s ComponentState) String() string {
	switch s {
	case StateUninitialized:
		return "uninitialized"
	case StateInitializing:
		return "initializing"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Component represents a managed component
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	State() ComponentState
}

// Coordinator coordinates component lifecycle
type Coordinator struct {
	mu         sync.RWMutex
	components []Component
	states     map[string]ComponentState
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewCoordinator creates a new component coordinator
func NewCoordinator(logger *zap.Logger) *Coordinator {
	return &Coordinator{
		components: make([]Component, 0),
		states:     make(map[string]ComponentState),
		logger:     logger,
	}
}

// Register registers a component
func (c *Coordinator) Register(component Component) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.components = append(c.components, component)
	c.states[component.Name()] = StateUninitialized
}

// StartAll starts all registered components
func (c *Coordinator) StartAll(ctx context.Context) error {
	c.mu.Lock()
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	for _, component := range c.components {
		c.setState(component.Name(), StateInitializing)

		c.logger.Info("starting component", zap.String("component", component.Name()))

		if err := component.Start(c.ctx); err != nil {
			c.setState(component.Name(), StateError)
			c.logger.Error("failed to start component",
				zap.String("component", component.Name()),
				zap.Error(err))
			return err
		}

		c.setState(component.Name(), StateRunning)
		c.logger.Info("component started", zap.String("component", component.Name()))
	}

	return nil
}

// StopAll stops all registered components in reverse order
func (c *Coordinator) StopAll(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()

	// Stop in reverse order
	for i := len(c.components) - 1; i >= 0; i-- {
		component := c.components[i]
		c.setState(component.Name(), StateStopping)

		c.logger.Info("stopping component", zap.String("component", component.Name()))

		if err := component.Stop(ctx); err != nil {
			c.logger.Error("failed to stop component",
				zap.String("component", component.Name()),
				zap.Error(err))
			// Continue stopping other components
		}

		c.setState(component.Name(), StateStopped)
		c.logger.Info("component stopped", zap.String("component", component.Name()))
	}

	return nil
}

// setState sets the state of a component
func (c *Coordinator) setState(name string, state ComponentState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[name] = state
}

// GetState returns the state of a component
func (c *Coordinator) GetState(name string) ComponentState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if state, ok := c.states[name]; ok {
		return state
	}
	return StateUninitialized
}

// GetAllStates returns all component states
func (c *Coordinator) GetAllStates() map[string]ComponentState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	states := make(map[string]ComponentState)
	for name, state := range c.states {
		states[name] = state
	}
	return states
}

// IsAllRunning returns true if all components are running
func (c *Coordinator) IsAllRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, state := range c.states {
		if state != StateRunning {
			return false
		}
	}
	return len(c.states) > 0
}

// WaitForAllRunning waits for all components to be running
func (c *Coordinator) WaitForAllRunning(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.IsAllRunning() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// RestartComponent restarts a specific component
func (c *Coordinator) RestartComponent(ctx context.Context, name string) error {
	c.mu.RLock()
	var component Component
	for _, comp := range c.components {
		if comp.Name() == name {
			component = comp
			break
		}
	}
	c.mu.RUnlock()

	if component == nil {
		return nil
	}

	c.setState(name, StateStopping)
	if err := component.Stop(ctx); err != nil {
		c.logger.Error("failed to stop component during restart",
			zap.String("component", name),
			zap.Error(err))
	}
	c.setState(name, StateStopped)

	c.setState(name, StateInitializing)
	if err := component.Start(c.ctx); err != nil {
		c.setState(name, StateError)
		return err
	}
	c.setState(name, StateRunning)

	return nil
}
