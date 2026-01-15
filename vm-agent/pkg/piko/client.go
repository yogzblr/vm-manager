// Package piko provides a client wrapper for connecting to Piko servers.
package piko

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Client represents a Piko client connection
type Client struct {
	mu          sync.RWMutex
	serverURL   string
	endpoint    string
	token       string
	tenantID    string
	conn        *websocket.Conn
	connected   bool
	lastError   error
	logger      *zap.Logger
	httpHandler http.Handler
	stopCh      chan struct{}
	wg          sync.WaitGroup
	reconnect   *ReconnectConfig
}

// ClientConfig contains client configuration
type ClientConfig struct {
	ServerURL   string
	Endpoint    string
	Token       string
	TenantID    string
	Reconnect   *ReconnectConfig
	HTTPHandler http.Handler
}

// NewClient creates a new Piko client
func NewClient(cfg *ClientConfig, logger *zap.Logger) *Client {
	reconnect := cfg.Reconnect
	if reconnect == nil {
		reconnect = DefaultReconnectConfig()
	}

	return &Client{
		serverURL:   cfg.ServerURL,
		endpoint:    cfg.Endpoint,
		token:       cfg.Token,
		tenantID:    cfg.TenantID,
		logger:      logger,
		httpHandler: cfg.HTTPHandler,
		reconnect:   reconnect,
		stopCh:      make(chan struct{}),
	}
}

// Start establishes the connection and starts handling requests
func (c *Client) Start(ctx context.Context) error {
	c.wg.Add(1)
	go c.connectionLoop(ctx)

	return nil
}

// Stop gracefully stops the client
func (c *Client) Stop() error {
	close(c.stopCh)
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.connected = false
	}

	return nil
}

// connectionLoop maintains the connection with automatic reconnection
func (c *Client) connectionLoop(ctx context.Context) {
	defer c.wg.Done()

	backoff := NewBackoff(c.reconnect)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		default:
		}

		err := c.connect(ctx)
		if err != nil {
			c.setError(err)
			c.logger.Error("failed to connect to Piko",
				zap.Error(err),
				zap.String("server_url", c.serverURL),
				zap.String("endpoint", c.endpoint))

			delay := backoff.Next()
			c.logger.Info("reconnecting after delay",
				zap.Duration("delay", delay))

			select {
			case <-ctx.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(delay):
				continue
			}
		}

		// Reset backoff on successful connection
		backoff.Reset()

		// Handle requests until disconnected
		c.handleRequests(ctx)
	}
}

// connect establishes the WebSocket connection to Piko
func (c *Client) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build the connection URL
	url := fmt.Sprintf("%s/piko/v1/upstream/%s", c.serverURL, c.endpoint)

	// Create headers
	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}
	if c.tenantID != "" {
		headers.Set("X-Piko-Tenant", c.tenantID)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("connection failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("connection failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	c.lastError = nil

	c.logger.Info("connected to Piko server",
		zap.String("endpoint", c.endpoint))

	return nil
}

// handleRequests handles incoming HTTP requests over the WebSocket
func (c *Client) handleRequests(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		// Read the message type and data
		messageType, reader, err := conn.NextReader()
		if err != nil {
			c.logger.Error("error reading from WebSocket", zap.Error(err))
			c.setConnected(false)
			return
		}

		if messageType != websocket.BinaryMessage {
			continue
		}

		// Handle the request in a goroutine
		go c.handleRequest(ctx, reader)
	}
}

// handleRequest handles a single HTTP request
func (c *Client) handleRequest(ctx context.Context, reader io.Reader) {
	// Read the request
	req, err := http.ReadRequest(newBufioReader(reader))
	if err != nil {
		c.logger.Error("error parsing HTTP request", zap.Error(err))
		return
	}

	// Create a response writer that captures the response
	rw := newResponseCapture()

	// Pass to the HTTP handler
	if c.httpHandler != nil {
		c.httpHandler.ServeHTTP(rw, req.WithContext(ctx))
	} else {
		http.Error(rw, "No handler configured", http.StatusServiceUnavailable)
	}

	// Send the response back through the WebSocket
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return
	}

	writer, err := conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		c.logger.Error("error getting WebSocket writer", zap.Error(err))
		return
	}
	defer writer.Close()

	if err := rw.WriteTo(writer); err != nil {
		c.logger.Error("error writing response", zap.Error(err))
	}
}

// setConnected sets the connection status
func (c *Client) setConnected(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = connected
	if !connected {
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
	}
}

// setError sets the last error
func (c *Client) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
	c.connected = false
}

// IsConnected returns true if connected to Piko
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// LastError returns the last connection error
func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// GetEndpoint returns the current endpoint
func (c *Client) GetEndpoint() string {
	return c.endpoint
}

// Listener returns a net.Listener that accepts connections from Piko
type Listener struct {
	client   *Client
	connCh   chan net.Conn
	closeCh  chan struct{}
	closeOnce sync.Once
}

// NewListener creates a new Piko listener
func NewListener(client *Client) *Listener {
	return &Listener{
		client:  client,
		connCh:  make(chan net.Conn, 100),
		closeCh: make(chan struct{}),
	}
}

// Accept waits for and returns the next connection
func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.closeCh:
		return nil, fmt.Errorf("listener closed")
	}
}

// Close closes the listener
func (l *Listener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closeCh)
	})
	return nil
}

// Addr returns the listener's network address
func (l *Listener) Addr() net.Addr {
	return &pikoAddr{endpoint: l.client.endpoint}
}

// pikoAddr implements net.Addr for Piko endpoints
type pikoAddr struct {
	endpoint string
}

func (a *pikoAddr) Network() string { return "piko" }
func (a *pikoAddr) String() string  { return a.endpoint }
