// Package audit provides audit logging with Quickwit integration.
package audit

import (
	"time"
)

// EventType represents the type of audit event
type EventType string

const (
	EventTypeAuth       EventType = "auth"
	EventTypeAgent      EventType = "agent"
	EventTypeWorkflow   EventType = "workflow"
	EventTypeCampaign   EventType = "campaign"
	EventTypeTenant     EventType = "tenant"
	EventTypeConfig     EventType = "config"
	EventTypeAPI        EventType = "api"
	EventTypeSystem     EventType = "system"
)

// EventAction represents the action performed
type EventAction string

const (
	ActionCreate   EventAction = "create"
	ActionRead     EventAction = "read"
	ActionUpdate   EventAction = "update"
	ActionDelete   EventAction = "delete"
	ActionStart    EventAction = "start"
	ActionStop     EventAction = "stop"
	ActionPause    EventAction = "pause"
	ActionResume   EventAction = "resume"
	ActionLogin    EventAction = "login"
	ActionLogout   EventAction = "logout"
	ActionRegister EventAction = "register"
	ActionExecute  EventAction = "execute"
	ActionRollback EventAction = "rollback"
)

// EventOutcome represents the outcome of the action
type EventOutcome string

const (
	OutcomeSuccess EventOutcome = "success"
	OutcomeFailure EventOutcome = "failure"
	OutcomeUnknown EventOutcome = "unknown"
)

// AuditEvent represents an audit log event
type AuditEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	TenantID    string                 `json:"tenant_id"`
	EventType   EventType              `json:"event_type"`
	Action      EventAction            `json:"action"`
	Outcome     EventOutcome           `json:"outcome"`
	ActorID     string                 `json:"actor_id,omitempty"`
	ActorType   string                 `json:"actor_type,omitempty"` // user, agent, system, api
	ResourceID  string                 `json:"resource_id,omitempty"`
	ResourceType string                `json:"resource_type,omitempty"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"`
	ErrorCode   string                 `json:"error_code,omitempty"`
	ErrorMsg    string                 `json:"error_message,omitempty"`
}

// QuickwitIndexConfig represents Quickwit index configuration
type QuickwitIndexConfig struct {
	Version          string           `json:"version"`
	IndexID          string           `json:"index_id"`
	DocMapping       DocMapping       `json:"doc_mapping"`
	SearchSettings   SearchSettings   `json:"search_settings"`
	IndexingSettings IndexingSettings `json:"indexing_settings"`
	RetentionPolicy  *RetentionPolicy `json:"retention_policy,omitempty"`
}

// DocMapping represents document mapping configuration
type DocMapping struct {
	Mode            string          `json:"mode"`
	FieldMappings   []FieldMapping  `json:"field_mappings"`
	TimestampField  string          `json:"timestamp_field"`
	TagFields       []string        `json:"tag_fields"`
	PartitionKey    string          `json:"partition_key,omitempty"`
}

// FieldMapping represents a field mapping
type FieldMapping struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Indexed    bool   `json:"indexed,omitempty"`
	Stored     bool   `json:"stored,omitempty"`
	Fast       bool   `json:"fast,omitempty"`
	Tokenizer  string `json:"tokenizer,omitempty"`
}

// SearchSettings represents search configuration
type SearchSettings struct {
	DefaultSearchFields []string `json:"default_search_fields"`
}

// IndexingSettings represents indexing configuration
type IndexingSettings struct {
	CommitTimeoutSecs   int `json:"commit_timeout_secs"`
	MergePolicy         MergePolicy `json:"merge_policy"`
	Resources           IndexingResources `json:"resources"`
}

// MergePolicy represents merge policy configuration
type MergePolicy struct {
	Type              string `json:"type"`
	MinMergeSegments  int    `json:"min_merge_segments,omitempty"`
	MergeFactor       int    `json:"merge_factor,omitempty"`
	MaxMergeSegments  int    `json:"max_merge_segments,omitempty"`
}

// IndexingResources represents indexing resources configuration
type IndexingResources struct {
	NumThreads int `json:"num_threads"`
	HeapSize   string `json:"heap_size"`
}

// RetentionPolicy represents data retention configuration
type RetentionPolicy struct {
	Period   string `json:"period"`
	Schedule string `json:"schedule"`
}

// DefaultAuditIndexConfig returns the default audit index configuration
func DefaultAuditIndexConfig(indexID string) *QuickwitIndexConfig {
	return &QuickwitIndexConfig{
		Version: "0.7",
		IndexID: indexID,
		DocMapping: DocMapping{
			Mode:           "dynamic",
			TimestampField: "timestamp",
			TagFields:      []string{"tenant_id", "event_type", "action", "outcome", "actor_type", "resource_type"},
			PartitionKey:   "tenant_id",
			FieldMappings: []FieldMapping{
				{Name: "id", Type: "text", Indexed: true, Stored: true},
				{Name: "timestamp", Type: "datetime", Indexed: true, Stored: true, Fast: true},
				{Name: "tenant_id", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "event_type", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "action", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "outcome", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "actor_id", Type: "text", Indexed: true, Stored: true},
				{Name: "actor_type", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "resource_id", Type: "text", Indexed: true, Stored: true},
				{Name: "resource_type", Type: "text", Indexed: true, Stored: true, Fast: true},
				{Name: "description", Type: "text", Indexed: true, Stored: true, Tokenizer: "default"},
				{Name: "ip_address", Type: "text", Indexed: true, Stored: true},
				{Name: "user_agent", Type: "text", Indexed: false, Stored: true},
				{Name: "request_id", Type: "text", Indexed: true, Stored: true},
				{Name: "duration_ms", Type: "i64", Indexed: true, Stored: true, Fast: true},
				{Name: "error_code", Type: "text", Indexed: true, Stored: true},
				{Name: "error_message", Type: "text", Indexed: true, Stored: true},
			},
		},
		SearchSettings: SearchSettings{
			DefaultSearchFields: []string{"description", "actor_id", "resource_id"},
		},
		IndexingSettings: IndexingSettings{
			CommitTimeoutSecs: 30,
			MergePolicy: MergePolicy{
				Type:             "log_merge",
				MinMergeSegments: 3,
				MergeFactor:      10,
				MaxMergeSegments: 10,
			},
			Resources: IndexingResources{
				NumThreads: 2,
				HeapSize:   "500MB",
			},
		},
		RetentionPolicy: &RetentionPolicy{
			Period:   "90 days",
			Schedule: "daily",
		},
	}
}

// SearchQuery represents a search query
type SearchQuery struct {
	Query       string            `json:"query"`
	TenantID    string            `json:"-"` // Used for filtering
	EventTypes  []EventType       `json:"-"`
	Actions     []EventAction     `json:"-"`
	Outcomes    []EventOutcome    `json:"-"`
	ActorID     string            `json:"-"`
	ResourceID  string            `json:"-"`
	StartTime   *time.Time        `json:"-"`
	EndTime     *time.Time        `json:"-"`
	MaxHits     int               `json:"max_hits"`
	StartOffset int               `json:"start_offset"`
	SortBy      []SortField       `json:"sort_by,omitempty"`
}

// SortField represents a sort field
type SortField struct {
	Field string `json:"field"`
	Order string `json:"order"` // asc, desc
}

// SearchResult represents search results
type SearchResult struct {
	Hits         []AuditEvent `json:"hits"`
	NumHits      int64        `json:"num_hits"`
	ElapsedSecs  float64      `json:"elapsed_secs"`
}
