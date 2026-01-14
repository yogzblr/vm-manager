// Package mcp provides MCP (Model Context Protocol) server implementation.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/agent"
	"github.com/yourorg/control-plane/pkg/audit"
	"github.com/yourorg/control-plane/pkg/campaign"
	"github.com/yourorg/control-plane/pkg/workflow"
)

// Server represents the MCP server
type Server struct {
	db              *gorm.DB
	logger          *zap.Logger
	agentRegistry   *agent.Registry
	workflowManager *workflow.Manager
	campaignManager *campaign.Manager
	auditLogger     *audit.Logger

	reader io.Reader
	writer io.Writer

	initialized bool
	mu          sync.RWMutex
}

// ServerConfig represents server configuration
type ServerConfig struct {
	DB              *gorm.DB
	Logger          *zap.Logger
	AgentRegistry   *agent.Registry
	WorkflowManager *workflow.Manager
	CampaignManager *campaign.Manager
	AuditLogger     *audit.Logger
}

// NewServer creates a new MCP server
func NewServer(config *ServerConfig) *Server {
	return &Server{
		db:              config.DB,
		logger:          config.Logger,
		agentRegistry:   config.AgentRegistry,
		workflowManager: config.WorkflowManager,
		campaignManager: config.CampaignManager,
		auditLogger:     config.AuditLogger,
		reader:          os.Stdin,
		writer:          os.Stdout,
	}
}

// SetIO sets custom input/output streams
func (s *Server) SetIO(reader io.Reader, writer io.Writer) {
	s.reader = reader
	s.writer = writer
}

// Run starts the MCP server main loop
func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("starting MCP server")

	scanner := bufio.NewScanner(s.reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		response := s.handleMessage(ctx, line)
		if response != nil {
			if err := s.writeResponse(response); err != nil {
				s.logger.Error("failed to write response", zap.Error(err))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

// handleMessage handles an incoming JSON-RPC message
func (s *Server) handleMessage(ctx context.Context, data []byte) *JSONRPCResponse {
	var request JSONRPCRequest
	if err := json.Unmarshal(data, &request); err != nil {
		s.logger.Error("failed to parse request", zap.Error(err))
		return NewErrorResponse(nil, ErrorCodeParseError, "Parse error", err.Error())
	}

	if request.JSONRPC != "2.0" {
		return NewErrorResponse(request.ID, ErrorCodeInvalidRequest, "Invalid request", "Invalid JSON-RPC version")
	}

	s.logger.Debug("received request", zap.String("method", request.Method))

	switch request.Method {
	case "initialize":
		return s.handleInitialize(ctx, &request)
	case "initialized":
		// Notification, no response needed
		return nil
	case "tools/list":
		return s.handleToolsList(ctx, &request)
	case "tools/call":
		return s.handleToolsCall(ctx, &request)
	case "resources/list":
		return s.handleResourcesList(ctx, &request)
	case "resources/read":
		return s.handleResourcesRead(ctx, &request)
	case "prompts/list":
		return s.handlePromptsList(ctx, &request)
	case "prompts/get":
		return s.handlePromptsGet(ctx, &request)
	case "ping":
		return NewSuccessResponse(request.ID, map[string]interface{}{})
	default:
		return NewErrorResponse(request.ID, ErrorCodeMethodNotFound, "Method not found", request.Method)
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	var params InitializeRequest
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return NewErrorResponse(request.ID, ErrorCodeInvalidParams, "Invalid params", err.Error())
	}

	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	s.logger.Info("client initialized",
		zap.String("client_name", params.ClientInfo.Name),
		zap.String("client_version", params.ClientInfo.Version),
		zap.String("protocol_version", params.ProtocolVersion))

	result := &InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
			Resources: &ResourcesCapability{
				Subscribe:   false,
				ListChanged: false,
			},
			Prompts: &PromptsCapability{
				ListChanged: false,
			},
			Logging: &LoggingCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Instructions: `VM Manager MCP Server - Use these tools to manage VMs, workflows, and campaigns.
Available operations:
- List and get agent information
- Create and manage workflows
- Create and execute campaigns for phased rollouts
- Search audit logs
- Generate workflow definitions from natural language`,
	}

	return NewSuccessResponse(request.ID, result)
}

// handleToolsList handles the tools/list request
func (s *Server) handleToolsList(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	tools := GetToolDefinitions()
	return NewSuccessResponse(request.ID, &ToolsListResult{Tools: tools})
}

// handleToolsCall handles the tools/call request
func (s *Server) handleToolsCall(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	initialized := s.initialized
	s.mu.RUnlock()

	if !initialized {
		return NewErrorResponse(request.ID, ErrorCodeInvalidRequest, "Server not initialized", nil)
	}

	var params CallToolRequest
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return NewErrorResponse(request.ID, ErrorCodeInvalidParams, "Invalid params", err.Error())
	}

	handler := NewToolHandler(s.db, s.logger, s.agentRegistry, s.workflowManager, s.campaignManager, s.auditLogger)
	result, err := handler.HandleTool(ctx, params.Name, params.Arguments)
	if err != nil {
		return NewSuccessResponse(request.ID, &CallToolResult{
			Content: []Content{TextContent(fmt.Sprintf("Error: %s", err.Error()))},
			IsError: true,
		})
	}

	return NewSuccessResponse(request.ID, result)
}

// handleResourcesList handles the resources/list request
func (s *Server) handleResourcesList(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	resources := []Resource{
		{
			URI:         "vmmanager://workflows",
			Name:        "Workflow Templates",
			Description: "Available workflow templates",
			MimeType:    "application/json",
		},
		{
			URI:         "vmmanager://agents",
			Name:        "Agent Status",
			Description: "Current status of all agents",
			MimeType:    "application/json",
		},
	}

	return NewSuccessResponse(request.ID, &ResourcesListResult{Resources: resources})
}

// handleResourcesRead handles the resources/read request
func (s *Server) handleResourcesRead(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	var params ReadResourceRequest
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return NewErrorResponse(request.ID, ErrorCodeInvalidParams, "Invalid params", err.Error())
	}

	// For now, return placeholder content
	return NewSuccessResponse(request.ID, &ReadResourceResult{
		Contents: []ResourceContent{
			{
				URI:      params.URI,
				MimeType: "application/json",
				Text:     `{"message": "Resource content placeholder"}`,
			},
		},
	})
}

// handlePromptsList handles the prompts/list request
func (s *Server) handlePromptsList(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	prompts := []Prompt{
		{
			Name:        "create_update_workflow",
			Description: "Generate a workflow for updating software on VMs",
			Arguments: []PromptArgument{
				{Name: "software_name", Description: "Name of the software to update", Required: true},
				{Name: "target_version", Description: "Target version to install", Required: true},
				{Name: "target_os", Description: "Target operating system (linux/windows)", Required: false},
			},
		},
		{
			Name:        "create_health_check_workflow",
			Description: "Generate a workflow for running health checks on VMs",
			Arguments: []PromptArgument{
				{Name: "check_type", Description: "Type of health check (disk, memory, cpu, network)", Required: true},
			},
		},
		{
			Name:        "create_rollout_campaign",
			Description: "Generate a campaign configuration for phased rollout",
			Arguments: []PromptArgument{
				{Name: "workflow_name", Description: "Name of the workflow to roll out", Required: true},
				{Name: "target_count", Description: "Approximate number of target agents", Required: true},
				{Name: "risk_level", Description: "Risk level (low/medium/high)", Required: false},
			},
		},
	}

	return NewSuccessResponse(request.ID, &PromptsListResult{Prompts: prompts})
}

// handlePromptsGet handles the prompts/get request
func (s *Server) handlePromptsGet(ctx context.Context, request *JSONRPCRequest) *JSONRPCResponse {
	var params GetPromptRequest
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return NewErrorResponse(request.ID, ErrorCodeInvalidParams, "Invalid params", err.Error())
	}

	var result *GetPromptResult

	switch params.Name {
	case "create_update_workflow":
		result = s.generateUpdateWorkflowPrompt(params.Arguments)
	case "create_health_check_workflow":
		result = s.generateHealthCheckPrompt(params.Arguments)
	case "create_rollout_campaign":
		result = s.generateRolloutCampaignPrompt(params.Arguments)
	default:
		return NewErrorResponse(request.ID, ErrorCodeInvalidParams, "Unknown prompt", params.Name)
	}

	return NewSuccessResponse(request.ID, result)
}

func (s *Server) generateUpdateWorkflowPrompt(args map[string]string) *GetPromptResult {
	softwareName := args["software_name"]
	targetVersion := args["target_version"]
	targetOS := args["target_os"]
	if targetOS == "" {
		targetOS = "linux"
	}

	return &GetPromptResult{
		Description: fmt.Sprintf("Workflow for updating %s to version %s", softwareName, targetVersion),
		Messages: []PromptMessage{
			{
				Role: "user",
				Content: TextContent(fmt.Sprintf(`Generate a VM Manager workflow YAML for updating %s to version %s on %s systems.

The workflow should include:
1. Pre-update checks (disk space, current version)
2. Backup of current configuration
3. Update/installation steps
4. Post-update verification
5. Rollback steps in case of failure

Use the standard VM Manager workflow format with:
- name, description fields
- steps array with step names, commands, and optional conditions
- Support for retry on failure
- Appropriate timeout values`, softwareName, targetVersion, targetOS)),
			},
		},
	}
}

func (s *Server) generateHealthCheckPrompt(args map[string]string) *GetPromptResult {
	checkType := args["check_type"]

	return &GetPromptResult{
		Description: fmt.Sprintf("Health check workflow for %s monitoring", checkType),
		Messages: []PromptMessage{
			{
				Role: "user",
				Content: TextContent(fmt.Sprintf(`Generate a VM Manager workflow YAML for performing %s health checks.

The workflow should:
1. Collect relevant metrics
2. Compare against thresholds
3. Report status (healthy/degraded/unhealthy)
4. Include remediation suggestions

Make sure the workflow works on both Linux and Windows systems where applicable.`, checkType)),
			},
		},
	}
}

func (s *Server) generateRolloutCampaignPrompt(args map[string]string) *GetPromptResult {
	workflowName := args["workflow_name"]
	targetCount := args["target_count"]
	riskLevel := args["risk_level"]
	if riskLevel == "" {
		riskLevel = "medium"
	}

	return &GetPromptResult{
		Description: fmt.Sprintf("Campaign configuration for %s rollout", workflowName),
		Messages: []PromptMessage{
			{
				Role: "user",
				Content: TextContent(fmt.Sprintf(`Generate a campaign configuration for rolling out the %s workflow to approximately %s agents.

Risk level: %s

Please recommend:
1. Number and size of rollout phases (canary, pilot, waves)
2. Success threshold for each phase
3. Wait time between phases
4. Target selector recommendations

Consider:
- Smaller canary for high-risk changes
- Longer wait times between phases for critical systems
- Success thresholds based on historical data`, workflowName, targetCount, riskLevel)),
			},
		},
	}
}

// writeResponse writes a response to the output stream
func (s *Server) writeResponse(response *JSONRPCResponse) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	data = append(data, '\n')
	_, err = s.writer.Write(data)
	return err
}
