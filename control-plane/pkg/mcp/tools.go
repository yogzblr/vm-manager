// Package mcp provides MCP (Model Context Protocol) server implementation.
package mcp

// Tool definitions for the VM Manager MCP server

// GetToolDefinitions returns all available tool definitions
func GetToolDefinitions() []Tool {
	return []Tool{
		listAgentsTool(),
		getAgentTool(),
		listWorkflowsTool(),
		getWorkflowTool(),
		createWorkflowTool(),
		executeWorkflowTool(),
		listCampaignsTool(),
		getCampaignTool(),
		createCampaignTool(),
		startCampaignTool(),
		getCampaignProgressTool(),
		searchAuditLogsTool(),
		generateWorkflowTool(),
	}
}

func listAgentsTool() Tool {
	return Tool{
		Name:        "list_agents",
		Description: "List all agents for a tenant with optional filtering by status and tags",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by agent status (online, offline, degraded)",
					"enum":        []string{"online", "offline", "degraded"},
				},
				"tags": map[string]interface{}{
					"type":        "object",
					"description": "Filter by tags (key-value pairs)",
					"additionalProperties": map[string]interface{}{
						"type": "string",
					},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of agents to return",
					"default":     50,
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Offset for pagination",
					"default":     0,
				},
			},
			"required": []string{"tenant_id"},
		},
	}
}

func getAgentTool() Tool {
	return Tool{
		Name:        "get_agent",
		Description: "Get detailed information about a specific agent",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "The agent ID",
				},
			},
			"required": []string{"tenant_id", "agent_id"},
		},
	}
}

func listWorkflowsTool() Tool {
	return Tool{
		Name:        "list_workflows",
		Description: "List all workflows for a tenant",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by workflow status",
					"enum":        []string{"draft", "active", "deprecated"},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of workflows to return",
					"default":     50,
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Offset for pagination",
					"default":     0,
				},
			},
			"required": []string{"tenant_id"},
		},
	}
}

func getWorkflowTool() Tool {
	return Tool{
		Name:        "get_workflow",
		Description: "Get detailed information about a specific workflow including its YAML definition",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"workflow_id": map[string]interface{}{
					"type":        "string",
					"description": "The workflow ID",
				},
			},
			"required": []string{"tenant_id", "workflow_id"},
		},
	}
}

func createWorkflowTool() Tool {
	return Tool{
		Name:        "create_workflow",
		Description: "Create a new workflow from a YAML definition",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The workflow name",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "The workflow description",
				},
				"definition": map[string]interface{}{
					"type":        "string",
					"description": "The workflow definition in YAML format",
				},
			},
			"required": []string{"tenant_id", "name", "definition"},
		},
	}
}

func executeWorkflowTool() Tool {
	return Tool{
		Name:        "execute_workflow",
		Description: "Execute a workflow on a specific agent",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"workflow_id": map[string]interface{}{
					"type":        "string",
					"description": "The workflow ID to execute",
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "The agent ID to execute on",
				},
				"parameters": map[string]interface{}{
					"type":        "object",
					"description": "Parameters to pass to the workflow",
					"additionalProperties": true,
				},
			},
			"required": []string{"tenant_id", "workflow_id", "agent_id"},
		},
	}
}

func listCampaignsTool() Tool {
	return Tool{
		Name:        "list_campaigns",
		Description: "List all campaigns for a tenant",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Filter by campaign status",
					"enum":        []string{"draft", "running", "paused", "completed", "failed", "cancelled"},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of campaigns to return",
					"default":     50,
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Offset for pagination",
					"default":     0,
				},
			},
			"required": []string{"tenant_id"},
		},
	}
}

func getCampaignTool() Tool {
	return Tool{
		Name:        "get_campaign",
		Description: "Get detailed information about a specific campaign",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"campaign_id": map[string]interface{}{
					"type":        "string",
					"description": "The campaign ID",
				},
			},
			"required": []string{"tenant_id", "campaign_id"},
		},
	}
}

func createCampaignTool() Tool {
	return Tool{
		Name:        "create_campaign",
		Description: "Create a new campaign for phased workflow rollout",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"workflow_id": map[string]interface{}{
					"type":        "string",
					"description": "The workflow ID to execute",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The campaign name",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "The campaign description",
				},
				"target_selector": map[string]interface{}{
					"type":        "object",
					"description": "Selector for target agents (tags, status, etc.)",
					"properties": map[string]interface{}{
						"tags": map[string]interface{}{
							"type": "object",
							"additionalProperties": map[string]interface{}{
								"type": "string",
							},
						},
						"status": map[string]interface{}{
							"type": "string",
						},
					},
				},
				"phases": map[string]interface{}{
					"type":        "array",
					"description": "Rollout phases configuration",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Phase name (e.g., canary, pilot, wave1)",
							},
							"percentage": map[string]interface{}{
								"type":        "number",
								"description": "Percentage of agents to target (0-100)",
							},
							"success_threshold": map[string]interface{}{
								"type":        "number",
								"description": "Success rate threshold to proceed (0-100)",
								"default":     95,
							},
							"wait_minutes": map[string]interface{}{
								"type":        "integer",
								"description": "Minutes to wait after phase completion",
								"default":     15,
							},
						},
						"required": []string{"name", "percentage"},
					},
				},
			},
			"required": []string{"tenant_id", "workflow_id", "name", "target_selector", "phases"},
		},
	}
}

func startCampaignTool() Tool {
	return Tool{
		Name:        "start_campaign",
		Description: "Start a campaign",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"campaign_id": map[string]interface{}{
					"type":        "string",
					"description": "The campaign ID to start",
				},
			},
			"required": []string{"tenant_id", "campaign_id"},
		},
	}
}

func getCampaignProgressTool() Tool {
	return Tool{
		Name:        "get_campaign_progress",
		Description: "Get the current progress of a campaign",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"campaign_id": map[string]interface{}{
					"type":        "string",
					"description": "The campaign ID",
				},
			},
			"required": []string{"tenant_id", "campaign_id"},
		},
	}
}

func searchAuditLogsTool() Tool {
	return Tool{
		Name:        "search_audit_logs",
		Description: "Search audit logs with various filters",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tenant_id": map[string]interface{}{
					"type":        "string",
					"description": "The tenant ID",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Free-text search query",
				},
				"event_types": map[string]interface{}{
					"type":        "array",
					"description": "Filter by event types",
					"items": map[string]interface{}{
						"type": "string",
						"enum": []string{"auth", "agent", "workflow", "campaign", "tenant", "config", "api", "system"},
					},
				},
				"actions": map[string]interface{}{
					"type":        "array",
					"description": "Filter by actions",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"actor_id": map[string]interface{}{
					"type":        "string",
					"description": "Filter by actor ID",
				},
				"resource_id": map[string]interface{}{
					"type":        "string",
					"description": "Filter by resource ID",
				},
				"start_time": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Start time for the search range (ISO 8601)",
				},
				"end_time": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "End time for the search range (ISO 8601)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results",
					"default":     100,
				},
			},
			"required": []string{"tenant_id"},
		},
	}
}

func generateWorkflowTool() Tool {
	return Tool{
		Name:        "generate_workflow",
		Description: "Generate a workflow YAML definition based on a natural language description. This helps create workflows for common operations.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Natural language description of what the workflow should do",
				},
				"target_os": map[string]interface{}{
					"type":        "string",
					"description": "Target operating system",
					"enum":        []string{"linux", "windows", "both"},
					"default":     "linux",
				},
				"include_rollback": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to include rollback steps",
					"default":     true,
				},
			},
			"required": []string{"description"},
		},
	}
}
