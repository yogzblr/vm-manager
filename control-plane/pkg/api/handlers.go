// Package api provides HTTP API handlers for the control plane.
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/yourorg/control-plane/pkg/agent"
	"github.com/yourorg/control-plane/pkg/audit"
	"github.com/yourorg/control-plane/pkg/auth"
	"github.com/yourorg/control-plane/pkg/campaign"
	"github.com/yourorg/control-plane/pkg/db/models"
	"github.com/yourorg/control-plane/pkg/template"
	"github.com/yourorg/control-plane/pkg/tenant"
	"github.com/yourorg/control-plane/pkg/workflow"
)

// Handlers contains all API handlers
type Handlers struct {
	logger          *zap.Logger
	tenantManager   *tenant.Manager
	agentRegistry   *agent.Registry
	agentRegistrar  *agent.Registrar
	workflowManager *workflow.Manager
	campaignManager *campaign.Manager
	templateManager *template.Manager
	auditLogger     *audit.Logger
}

// NewHandlers creates new API handlers
func NewHandlers(
	logger *zap.Logger,
	tenantManager *tenant.Manager,
	agentRegistry *agent.Registry,
	agentRegistrar *agent.Registrar,
	workflowManager *workflow.Manager,
	campaignManager *campaign.Manager,
	templateManager *template.Manager,
	auditLogger *audit.Logger,
) *Handlers {
	return &Handlers{
		logger:          logger,
		tenantManager:   tenantManager,
		agentRegistry:   agentRegistry,
		agentRegistrar:  agentRegistrar,
		workflowManager: workflowManager,
		campaignManager: campaignManager,
		templateManager: templateManager,
		auditLogger:     auditLogger,
	}
}

// Health check handlers

// HealthCheck returns the health status
func (h *Handlers) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

// Readiness returns the readiness status
func (h *Handlers) Readiness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ready": true,
	})
}

// Tenant handlers

// ListTenants lists all tenants
func (h *Handlers) ListTenants(c *gin.Context) {
	ctx := c.Request.Context()
	limit := getIntParam(c, "limit", 50)
	offset := getIntParam(c, "offset", 0)

	tenants, total, err := h.tenantManager.List(ctx, limit, offset)
	if err != nil {
		h.logger.Error("failed to list tenants", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenants": tenants,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetTenant gets a tenant by ID
func (h *Handlers) GetTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	t, err := h.tenantManager.Get(ctx, tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, t)
}

// CreateTenant creates a new tenant
func (h *Handlers) CreateTenant(c *gin.Context) {
	ctx := c.Request.Context()

	var req tenant.CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	t, err := h.tenantManager.Create(ctx, &req)
	if err != nil {
		h.logger.Error("failed to create tenant", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, t)
}

// UpdateTenant updates a tenant
func (h *Handlers) UpdateTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenant_id")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.tenantManager.Update(ctx, tenantID, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tenant updated"})
}

// Agent handlers

// ListAgents lists agents for a tenant
func (h *Handlers) ListAgents(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	status := c.Query("status")
	limit := getIntParam(c, "limit", 50)
	offset := getIntParam(c, "offset", 0)

	agents, total, err := h.agentRegistry.List(ctx, &agent.ListRequest{
		TenantID: tenantID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.logger.Error("failed to list agents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetAgent gets an agent by ID
func (h *Handlers) GetAgent(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	agentID := c.Param("agent_id")

	ag, err := h.agentRegistry.Get(ctx, tenantID, agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ag)
}

// RegisterAgent registers a new agent
func (h *Handlers) RegisterAgent(c *gin.Context) {
	ctx := c.Request.Context()

	var req agent.RegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.agentRegistrar.Register(ctx, &req)
	if err != nil {
		h.logger.Error("failed to register agent", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, result)
}

// AgentHeartbeat handles agent heartbeat
func (h *Handlers) AgentHeartbeat(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	agentID := c.Param("agent_id")

	if err := h.agentRegistry.UpdateHeartbeat(ctx, tenantID, agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "heartbeat recorded"})
}

// AgentHealthReport handles agent health reports
func (h *Handlers) AgentHealthReport(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	agentID := c.Param("agent_id")

	var req struct {
		Status     models.AgentStatus     `json:"status"`
		Components map[string]interface{} `json:"components"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.agentRegistry.RecordHealthReport(ctx, tenantID, agentID, req.Status, req.Components); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "health report recorded"})
}

// Workflow handlers

// ListWorkflows lists workflows for a tenant
func (h *Handlers) ListWorkflows(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	status := models.WorkflowStatus(c.Query("status"))
	limit := getIntParam(c, "limit", 50)
	offset := getIntParam(c, "offset", 0)

	workflows, total, err := h.workflowManager.List(ctx, tenantID, status, limit, offset)
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"workflows": workflows,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetWorkflow gets a workflow by ID
func (h *Handlers) GetWorkflow(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	workflowID := c.Param("workflow_id")

	wf, err := h.workflowManager.Get(ctx, tenantID, workflowID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, wf)
}

// CreateWorkflow creates a new workflow
func (h *Handlers) CreateWorkflow(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)

	var req workflow.CreateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.TenantID = tenantID

	// Get created by from auth context
	if claims, ok := c.Get("claims"); ok {
		if authClaims, ok := claims.(*auth.Claims); ok {
			req.CreatedBy = authClaims.UserID
		}
	}

	wf, err := h.workflowManager.Create(ctx, &req)
	if err != nil {
		h.logger.Error("failed to create workflow", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, wf)
}

// UpdateWorkflow updates a workflow
func (h *Handlers) UpdateWorkflow(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	workflowID := c.Param("workflow_id")

	var req workflow.UpdateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.TenantID = tenantID
	req.WorkflowID = workflowID

	if err := h.workflowManager.Update(ctx, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workflow updated"})
}

// DeleteWorkflow deletes a workflow
func (h *Handlers) DeleteWorkflow(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	workflowID := c.Param("workflow_id")

	if err := h.workflowManager.Delete(ctx, tenantID, workflowID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workflow deleted"})
}

// Campaign handlers

// ListCampaigns lists campaigns for a tenant
func (h *Handlers) ListCampaigns(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	status := models.CampaignStatus(c.Query("status"))
	limit := getIntParam(c, "limit", 50)
	offset := getIntParam(c, "offset", 0)

	campaigns, total, err := h.campaignManager.List(ctx, tenantID, status, limit, offset)
	if err != nil {
		h.logger.Error("failed to list campaigns", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"campaigns": campaigns,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetCampaign gets a campaign by ID
func (h *Handlers) GetCampaign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	campaignID := c.Param("campaign_id")

	camp, err := h.campaignManager.Get(ctx, tenantID, campaignID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, camp)
}

// CreateCampaign creates a new campaign
func (h *Handlers) CreateCampaign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)

	var req campaign.CreateCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.TenantID = tenantID

	// Get created by from auth context
	if claims, ok := c.Get("claims"); ok {
		if authClaims, ok := claims.(*auth.Claims); ok {
			req.CreatedBy = authClaims.UserID
		}
	}

	camp, err := h.campaignManager.Create(ctx, &req)
	if err != nil {
		h.logger.Error("failed to create campaign", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, camp)
}

// StartCampaign starts a campaign
func (h *Handlers) StartCampaign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	campaignID := c.Param("campaign_id")

	if err := h.campaignManager.Start(ctx, tenantID, campaignID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "campaign started"})
}

// PauseCampaign pauses a campaign
func (h *Handlers) PauseCampaign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	campaignID := c.Param("campaign_id")

	if err := h.campaignManager.Pause(ctx, tenantID, campaignID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "campaign paused"})
}

// CancelCampaign cancels a campaign
func (h *Handlers) CancelCampaign(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	campaignID := c.Param("campaign_id")

	if err := h.campaignManager.Cancel(ctx, tenantID, campaignID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "campaign cancelled"})
}

// GetCampaignProgress gets campaign progress
func (h *Handlers) GetCampaignProgress(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	campaignID := c.Param("campaign_id")

	progress, err := h.campaignManager.GetProgress(ctx, tenantID, campaignID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, progress)
}

// Template handlers

// ListTemplates lists templates for a tenant
func (h *Handlers) ListTemplates(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	status := models.TemplateStatus(c.Query("status"))
	limit := getIntParam(c, "limit", 50)
	offset := getIntParam(c, "offset", 0)

	templates, total, err := h.templateManager.List(ctx, &template.ListTemplatesRequest{
		TenantID: tenantID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		h.logger.Error("failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"templates": templates,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetTemplate gets a template by ID
func (h *Handlers) GetTemplate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	tpl, err := h.templateManager.Get(ctx, tenantID, templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tpl)
}

// GetTemplateContent gets raw template content (for agents to fetch)
func (h *Handlers) GetTemplateContent(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	content, err := h.templateManager.GetContent(ctx, tenantID, templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Return raw content with appropriate content type
	tpl, _ := h.templateManager.Get(ctx, tenantID, templateID)
	contentType := "text/plain"
	if tpl != nil && tpl.ContentType != "" {
		contentType = tpl.ContentType
	}

	c.Data(http.StatusOK, contentType, []byte(content))
}

// CreateTemplate creates a new template
func (h *Handlers) CreateTemplate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)

	var req template.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.TenantID = tenantID

	// Get created by from auth context
	if claims, ok := c.Get("claims"); ok {
		if authClaims, ok := claims.(*auth.Claims); ok {
			req.CreatedBy = authClaims.UserID
		}
	}

	tpl, err := h.templateManager.Create(ctx, &req)
	if err != nil {
		h.logger.Error("failed to create template", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tpl)
}

// UpdateTemplate updates a template
func (h *Handlers) UpdateTemplate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	var req template.UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get changed by from auth context
	if claims, ok := c.Get("claims"); ok {
		if authClaims, ok := claims.(*auth.Claims); ok {
			req.ChangedBy = authClaims.UserID
		}
	}

	tpl, err := h.templateManager.Update(ctx, tenantID, templateID, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tpl)
}

// DeleteTemplate deletes a template
func (h *Handlers) DeleteTemplate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	if err := h.templateManager.Delete(ctx, tenantID, templateID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "template deleted"})
}

// GetTemplateVersions gets all versions of a template
func (h *Handlers) GetTemplateVersions(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	versions, err := h.templateManager.GetVersions(ctx, tenantID, templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// ActivateTemplate activates a template
func (h *Handlers) ActivateTemplate(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getTenantID(c)
	templateID := c.Param("template_id")

	if err := h.templateManager.Activate(ctx, tenantID, templateID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "template activated"})
}

// Helper functions

func getTenantID(c *gin.Context) string {
	// First try to get from claims
	if claims, ok := c.Get("claims"); ok {
		if authClaims, ok := claims.(*auth.Claims); ok && authClaims.TenantID != "" {
			return authClaims.TenantID
		}
	}
	// Fall back to path parameter
	return c.Param("tenant_id")
}

func getIntParam(c *gin.Context, key string, defaultValue int) int {
	val := c.Query(key)
	if val == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return i
}
