// Package piko provides a client wrapper for connecting to Piko servers.
package piko

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Tunnel represents a bidirectional tunnel through Piko
type Tunnel struct {
	mu       sync.Mutex
	client   *Client
	endpoint string
	active   bool
}

// NewTunnel creates a new tunnel
func NewTunnel(client *Client, endpoint string) *Tunnel {
	return &Tunnel{
		client:   client,
		endpoint: endpoint,
	}
}

// Open opens the tunnel
func (t *Tunnel) Open() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.active {
		return nil
	}

	t.active = true
	return nil
}

// Close closes the tunnel
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.active = false
	return nil
}

// IsActive returns true if the tunnel is active
func (t *Tunnel) IsActive() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

// responseCapture captures an HTTP response for transmission
type responseCapture struct {
	status  int
	headers http.Header
	body    *bytes.Buffer
}

func newResponseCapture() *responseCapture {
	return &responseCapture{
		status:  http.StatusOK,
		headers: make(http.Header),
		body:    &bytes.Buffer{},
	}
}

// Header returns the header map
func (w *responseCapture) Header() http.Header {
	return w.headers
}

// Write writes data to the body
func (w *responseCapture) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

// WriteHeader sets the status code
func (w *responseCapture) WriteHeader(statusCode int) {
	w.status = statusCode
}

// WriteTo writes the captured response to a writer
func (w *responseCapture) WriteTo(writer io.Writer) error {
	// Write status line
	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", w.status, http.StatusText(w.status))
	if _, err := writer.Write([]byte(statusLine)); err != nil {
		return err
	}

	// Write headers
	for key, values := range w.headers {
		for _, value := range values {
			header := fmt.Sprintf("%s: %s\r\n", key, value)
			if _, err := writer.Write([]byte(header)); err != nil {
				return err
			}
		}
	}

	// Add Content-Length header if not present
	if w.headers.Get("Content-Length") == "" {
		header := fmt.Sprintf("Content-Length: %d\r\n", w.body.Len())
		if _, err := writer.Write([]byte(header)); err != nil {
			return err
		}
	}

	// End headers
	if _, err := writer.Write([]byte("\r\n")); err != nil {
		return err
	}

	// Write body
	if _, err := w.body.WriteTo(writer); err != nil {
		return err
	}

	return nil
}

// bufioReaderWrapper wraps an io.Reader with a bufio.Reader
type bufioReaderWrapper struct {
	*bufio.Reader
}

func newBufioReader(r io.Reader) *bufio.Reader {
	if br, ok := r.(*bufio.Reader); ok {
		return br
	}
	return bufio.NewReader(r)
}

// TunnelMetrics contains tunnel performance metrics
type TunnelMetrics struct {
	BytesSent       int64
	BytesReceived   int64
	RequestCount    int64
	ErrorCount      int64
	AverageLatency  float64
	ConnectionCount int64
}

// TunnelManager manages multiple tunnels
type TunnelManager struct {
	mu      sync.RWMutex
	tunnels map[string]*Tunnel
	client  *Client
}

// NewTunnelManager creates a new tunnel manager
func NewTunnelManager(client *Client) *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]*Tunnel),
		client:  client,
	}
}

// GetOrCreateTunnel gets or creates a tunnel for the given endpoint
func (m *TunnelManager) GetOrCreateTunnel(endpoint string) *Tunnel {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tunnel, ok := m.tunnels[endpoint]; ok {
		return tunnel
	}

	tunnel := NewTunnel(m.client, endpoint)
	m.tunnels[endpoint] = tunnel
	return tunnel
}

// GetTunnel returns a tunnel by endpoint
func (m *TunnelManager) GetTunnel(endpoint string) (*Tunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tunnel, ok := m.tunnels[endpoint]
	return tunnel, ok
}

// CloseTunnel closes a tunnel by endpoint
func (m *TunnelManager) CloseTunnel(endpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tunnel, ok := m.tunnels[endpoint]; ok {
		if err := tunnel.Close(); err != nil {
			return err
		}
		delete(m.tunnels, endpoint)
	}
	return nil
}

// CloseAll closes all tunnels
func (m *TunnelManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for endpoint, tunnel := range m.tunnels {
		tunnel.Close()
		delete(m.tunnels, endpoint)
	}
	return nil
}

// ListTunnels returns all active tunnel endpoints
func (m *TunnelManager) ListTunnels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoints := make([]string, 0, len(m.tunnels))
	for endpoint, tunnel := range m.tunnels {
		if tunnel.IsActive() {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
}
