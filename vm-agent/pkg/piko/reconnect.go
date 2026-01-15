// Package piko provides a client wrapper for connecting to Piko servers.
package piko

import (
	"math/rand"
	"sync"
	"time"
)

// ReconnectConfig contains reconnection configuration
type ReconnectConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       float64
}

// DefaultReconnectConfig returns the default reconnection configuration
func DefaultReconnectConfig() *ReconnectConfig {
	return &ReconnectConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1, // 10% jitter
	}
}

// Backoff implements exponential backoff with jitter
type Backoff struct {
	mu           sync.Mutex
	config       *ReconnectConfig
	currentDelay time.Duration
	attempts     int
}

// NewBackoff creates a new backoff calculator
func NewBackoff(config *ReconnectConfig) *Backoff {
	if config == nil {
		config = DefaultReconnectConfig()
	}
	return &Backoff{
		config:       config,
		currentDelay: config.InitialDelay,
	}
}

// Next returns the next backoff duration
func (b *Backoff) Next() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	delay := b.currentDelay
	b.attempts++

	// Apply jitter
	if b.config.Jitter > 0 {
		jitter := float64(delay) * b.config.Jitter * (rand.Float64()*2 - 1)
		delay = time.Duration(float64(delay) + jitter)
	}

	// Calculate next delay
	b.currentDelay = time.Duration(float64(b.currentDelay) * b.config.Multiplier)
	if b.currentDelay > b.config.MaxDelay {
		b.currentDelay = b.config.MaxDelay
	}

	return delay
}

// Reset resets the backoff to initial state
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.currentDelay = b.config.InitialDelay
	b.attempts = 0
}

// Attempts returns the number of attempts made
func (b *Backoff) Attempts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.attempts
}

// CurrentDelay returns the current delay value
func (b *Backoff) CurrentDelay() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentDelay
}

// ReconnectStrategy defines reconnection behavior
type ReconnectStrategy interface {
	ShouldReconnect(attempts int, err error) bool
	NextDelay(attempts int) time.Duration
	OnSuccess()
	OnFailure(err error)
}

// ExponentialBackoffStrategy implements exponential backoff reconnection
type ExponentialBackoffStrategy struct {
	backoff     *Backoff
	maxAttempts int
}

// NewExponentialBackoffStrategy creates a new exponential backoff strategy
func NewExponentialBackoffStrategy(config *ReconnectConfig, maxAttempts int) *ExponentialBackoffStrategy {
	return &ExponentialBackoffStrategy{
		backoff:     NewBackoff(config),
		maxAttempts: maxAttempts,
	}
}

// ShouldReconnect returns true if reconnection should be attempted
func (s *ExponentialBackoffStrategy) ShouldReconnect(attempts int, err error) bool {
	if s.maxAttempts > 0 && attempts >= s.maxAttempts {
		return false
	}
	return true
}

// NextDelay returns the next delay duration
func (s *ExponentialBackoffStrategy) NextDelay(attempts int) time.Duration {
	return s.backoff.Next()
}

// OnSuccess is called when connection succeeds
func (s *ExponentialBackoffStrategy) OnSuccess() {
	s.backoff.Reset()
}

// OnFailure is called when connection fails
func (s *ExponentialBackoffStrategy) OnFailure(err error) {
	// No-op for basic strategy
}

// ConnectionState represents the state of a connection
type ConnectionState int

const (
	// StateDisconnected indicates no active connection
	StateDisconnected ConnectionState = iota
	// StateConnecting indicates connection in progress
	StateConnecting
	// StateConnected indicates active connection
	StateConnected
	// StateReconnecting indicates reconnection in progress
	StateReconnecting
)

// String returns the string representation of the state
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// ConnectionStateListener is notified of connection state changes
type ConnectionStateListener interface {
	OnStateChange(old, new ConnectionState)
}

// ConnectionManager manages connection state and reconnection
type ConnectionManager struct {
	mu        sync.RWMutex
	state     ConnectionState
	strategy  ReconnectStrategy
	listeners []ConnectionStateListener
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(strategy ReconnectStrategy) *ConnectionManager {
	return &ConnectionManager{
		state:     StateDisconnected,
		strategy:  strategy,
		listeners: make([]ConnectionStateListener, 0),
	}
}

// State returns the current connection state
func (m *ConnectionManager) State() ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// SetState sets the connection state
func (m *ConnectionManager) SetState(state ConnectionState) {
	m.mu.Lock()
	oldState := m.state
	m.state = state
	listeners := m.listeners
	m.mu.Unlock()

	if oldState != state {
		for _, listener := range listeners {
			listener.OnStateChange(oldState, state)
		}
	}
}

// AddListener adds a state change listener
func (m *ConnectionManager) AddListener(listener ConnectionStateListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

// IsConnected returns true if currently connected
func (m *ConnectionManager) IsConnected() bool {
	return m.State() == StateConnected
}
