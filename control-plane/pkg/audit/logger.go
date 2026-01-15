// Package audit provides audit logging with Quickwit integration.
package audit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Logger provides audit logging functionality
type Logger struct {
	client        *QuickwitClient
	logger        *zap.Logger
	config        *QuickwitConfig

	// Batching
	mu            sync.Mutex
	batch         []AuditEvent
	flushTicker   *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewLogger creates a new audit logger
func NewLogger(client *QuickwitClient, config *QuickwitConfig, logger *zap.Logger) *Logger {
	l := &Logger{
		client:   client,
		logger:   logger,
		config:   config,
		batch:    make([]AuditEvent, 0, config.BatchSize),
		stopChan: make(chan struct{}),
	}

	if config.EnableBatch {
		l.startBatchProcessor()
	}

	return l
}

// startBatchProcessor starts the background batch processor
func (l *Logger) startBatchProcessor() {
	l.flushTicker = time.NewTicker(l.config.FlushInterval)
	l.wg.Add(1)

	go func() {
		defer l.wg.Done()
		for {
			select {
			case <-l.flushTicker.C:
				if err := l.Flush(context.Background()); err != nil {
					l.logger.Error("failed to flush audit logs", zap.Error(err))
				}
			case <-l.stopChan:
				return
			}
		}
	}()
}

// Close stops the logger and flushes remaining events
func (l *Logger) Close() error {
	if l.flushTicker != nil {
		l.flushTicker.Stop()
	}

	close(l.stopChan)
	l.wg.Wait()

	// Final flush
	return l.Flush(context.Background())
}

// Log logs an audit event
func (l *Logger) Log(ctx context.Context, event *AuditEvent) error {
	// Generate ID if not set
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Set default outcome
	if event.Outcome == "" {
		event.Outcome = OutcomeSuccess
	}

	if l.config.EnableBatch {
		return l.addToBatch(ctx, event)
	}

	return l.client.IngestSingle(ctx, event)
}

// addToBatch adds an event to the batch
func (l *Logger) addToBatch(ctx context.Context, event *AuditEvent) error {
	l.mu.Lock()
	l.batch = append(l.batch, *event)
	shouldFlush := len(l.batch) >= l.config.BatchSize
	l.mu.Unlock()

	if shouldFlush {
		return l.Flush(ctx)
	}

	return nil
}

// Flush flushes the current batch
func (l *Logger) Flush(ctx context.Context) error {
	l.mu.Lock()
	if len(l.batch) == 0 {
		l.mu.Unlock()
		return nil
	}

	batch := l.batch
	l.batch = make([]AuditEvent, 0, l.config.BatchSize)
	l.mu.Unlock()

	if err := l.client.Ingest(ctx, batch); err != nil {
		// Put events back in batch on failure
		l.mu.Lock()
		l.batch = append(batch, l.batch...)
		l.mu.Unlock()
		return err
	}

	l.logger.Debug("flushed audit logs", zap.Int("count", len(batch)))
	return nil
}

// LogAuth logs an authentication event
func (l *Logger) LogAuth(ctx context.Context, tenantID, actorID, actorType, action string, success bool, metadata map[string]interface{}) error {
	outcome := OutcomeSuccess
	if !success {
		outcome = OutcomeFailure
	}

	return l.Log(ctx, &AuditEvent{
		TenantID:  tenantID,
		EventType: EventTypeAuth,
		Action:    EventAction(action),
		Outcome:   outcome,
		ActorID:   actorID,
		ActorType: actorType,
		Metadata:  metadata,
	})
}

// LogAgentEvent logs an agent-related event
func (l *Logger) LogAgentEvent(ctx context.Context, tenantID, agentID, action string, success bool, metadata map[string]interface{}) error {
	outcome := OutcomeSuccess
	if !success {
		outcome = OutcomeFailure
	}

	return l.Log(ctx, &AuditEvent{
		TenantID:     tenantID,
		EventType:    EventTypeAgent,
		Action:       EventAction(action),
		Outcome:      outcome,
		ResourceID:   agentID,
		ResourceType: "agent",
		Metadata:     metadata,
	})
}

// LogWorkflowEvent logs a workflow-related event
func (l *Logger) LogWorkflowEvent(ctx context.Context, tenantID, workflowID, action, actorID string, success bool, metadata map[string]interface{}) error {
	outcome := OutcomeSuccess
	if !success {
		outcome = OutcomeFailure
	}

	return l.Log(ctx, &AuditEvent{
		TenantID:     tenantID,
		EventType:    EventTypeWorkflow,
		Action:       EventAction(action),
		Outcome:      outcome,
		ActorID:      actorID,
		ResourceID:   workflowID,
		ResourceType: "workflow",
		Metadata:     metadata,
	})
}

// LogCampaignEvent logs a campaign-related event
func (l *Logger) LogCampaignEvent(ctx context.Context, tenantID, campaignID, action, actorID string, success bool, metadata map[string]interface{}) error {
	outcome := OutcomeSuccess
	if !success {
		outcome = OutcomeFailure
	}

	return l.Log(ctx, &AuditEvent{
		TenantID:     tenantID,
		EventType:    EventTypeCampaign,
		Action:       EventAction(action),
		Outcome:      outcome,
		ActorID:      actorID,
		ResourceID:   campaignID,
		ResourceType: "campaign",
		Metadata:     metadata,
	})
}

// LogAPIRequest logs an API request
func (l *Logger) LogAPIRequest(ctx context.Context, tenantID, actorID, actorType, method, path string, statusCode int, duration time.Duration, metadata map[string]interface{}) error {
	outcome := OutcomeSuccess
	if statusCode >= 400 {
		outcome = OutcomeFailure
	}

	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["method"] = method
	metadata["path"] = path
	metadata["status_code"] = statusCode

	return l.Log(ctx, &AuditEvent{
		TenantID:  tenantID,
		EventType: EventTypeAPI,
		Action:    ActionRead,
		Outcome:   outcome,
		ActorID:   actorID,
		ActorType: actorType,
		Duration:  duration.Milliseconds(),
		Metadata:  metadata,
	})
}

// LogSystemEvent logs a system event
func (l *Logger) LogSystemEvent(ctx context.Context, action, description string, metadata map[string]interface{}) error {
	return l.Log(ctx, &AuditEvent{
		TenantID:    "system",
		EventType:   EventTypeSystem,
		Action:      EventAction(action),
		Outcome:     OutcomeSuccess,
		ActorType:   "system",
		Description: description,
		Metadata:    metadata,
	})
}

// Search searches audit logs
func (l *Logger) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	return l.client.Search(ctx, query)
}

// GetEventsByTenant gets events for a specific tenant
func (l *Logger) GetEventsByTenant(ctx context.Context, tenantID string, limit, offset int) (*SearchResult, error) {
	return l.client.Search(ctx, &SearchQuery{
		TenantID:    tenantID,
		MaxHits:     limit,
		StartOffset: offset,
		SortBy: []SortField{
			{Field: "timestamp", Order: "desc"},
		},
	})
}

// GetEventsByActor gets events for a specific actor
func (l *Logger) GetEventsByActor(ctx context.Context, tenantID, actorID string, limit, offset int) (*SearchResult, error) {
	return l.client.Search(ctx, &SearchQuery{
		TenantID:    tenantID,
		ActorID:     actorID,
		MaxHits:     limit,
		StartOffset: offset,
		SortBy: []SortField{
			{Field: "timestamp", Order: "desc"},
		},
	})
}

// GetEventsByResource gets events for a specific resource
func (l *Logger) GetEventsByResource(ctx context.Context, tenantID, resourceID string, limit, offset int) (*SearchResult, error) {
	return l.client.Search(ctx, &SearchQuery{
		TenantID:   tenantID,
		ResourceID: resourceID,
		MaxHits:    limit,
		StartOffset: offset,
		SortBy: []SortField{
			{Field: "timestamp", Order: "desc"},
		},
	})
}

// GetAggregatedCounts gets aggregated counts by field
func (l *Logger) GetAggregatedCounts(ctx context.Context, tenantID, field string, startTime, endTime *time.Time) (map[string]int64, error) {
	return l.client.Aggregate(ctx, tenantID, field, startTime, endTime)
}

// EnsureIndex ensures the audit index exists
func (l *Logger) EnsureIndex(ctx context.Context) error {
	exists, err := l.client.IndexExists(ctx, l.config.IndexID)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}

	if !exists {
		config := DefaultAuditIndexConfig(l.config.IndexID)
		if err := l.client.CreateIndex(ctx, config); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// AuditMiddlewareData represents data extracted from request context for audit logging
type AuditMiddlewareData struct {
	TenantID   string
	ActorID    string
	ActorType  string
	IPAddress  string
	UserAgent  string
	RequestID  string
}

// ExtractFromContext extracts audit data from context
// This should be customized based on your context setup
func ExtractFromContext(ctx context.Context) *AuditMiddlewareData {
	// This is a placeholder - actual implementation depends on your context setup
	return &AuditMiddlewareData{}
}

// NewEventBuilder creates a new event builder for fluent API
func (l *Logger) NewEventBuilder() *EventBuilder {
	return &EventBuilder{
		logger: l,
		event:  &AuditEvent{},
	}
}

// EventBuilder provides a fluent API for building audit events
type EventBuilder struct {
	logger *Logger
	event  *AuditEvent
}

// WithTenant sets the tenant ID
func (b *EventBuilder) WithTenant(tenantID string) *EventBuilder {
	b.event.TenantID = tenantID
	return b
}

// WithType sets the event type
func (b *EventBuilder) WithType(eventType EventType) *EventBuilder {
	b.event.EventType = eventType
	return b
}

// WithAction sets the action
func (b *EventBuilder) WithAction(action EventAction) *EventBuilder {
	b.event.Action = action
	return b
}

// WithOutcome sets the outcome
func (b *EventBuilder) WithOutcome(outcome EventOutcome) *EventBuilder {
	b.event.Outcome = outcome
	return b
}

// WithActor sets the actor
func (b *EventBuilder) WithActor(actorID, actorType string) *EventBuilder {
	b.event.ActorID = actorID
	b.event.ActorType = actorType
	return b
}

// WithResource sets the resource
func (b *EventBuilder) WithResource(resourceID, resourceType string) *EventBuilder {
	b.event.ResourceID = resourceID
	b.event.ResourceType = resourceType
	return b
}

// WithDescription sets the description
func (b *EventBuilder) WithDescription(description string) *EventBuilder {
	b.event.Description = description
	return b
}

// WithMetadata sets metadata
func (b *EventBuilder) WithMetadata(metadata map[string]interface{}) *EventBuilder {
	b.event.Metadata = metadata
	return b
}

// WithRequestInfo sets request information
func (b *EventBuilder) WithRequestInfo(ipAddress, userAgent, requestID string) *EventBuilder {
	b.event.IPAddress = ipAddress
	b.event.UserAgent = userAgent
	b.event.RequestID = requestID
	return b
}

// WithDuration sets the duration
func (b *EventBuilder) WithDuration(duration time.Duration) *EventBuilder {
	b.event.Duration = duration.Milliseconds()
	return b
}

// WithError sets error information
func (b *EventBuilder) WithError(code, message string) *EventBuilder {
	b.event.ErrorCode = code
	b.event.ErrorMsg = message
	b.event.Outcome = OutcomeFailure
	return b
}

// Log logs the event
func (b *EventBuilder) Log(ctx context.Context) error {
	return b.logger.Log(ctx, b.event)
}
