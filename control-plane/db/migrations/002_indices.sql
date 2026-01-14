-- Performance indices for Multi-Tenant VM Manager

-- Tenant indices
CREATE INDEX idx_tenants_status ON tenants(status);
CREATE INDEX idx_tenants_created_at ON tenants(created_at);

-- Tenant API keys indices
CREATE INDEX idx_tenant_api_keys_tenant_id ON tenant_api_keys(tenant_id);
CREATE INDEX idx_tenant_api_keys_key_hash ON tenant_api_keys(key_hash);

-- Installation keys indices
CREATE INDEX idx_installation_keys_tenant_id ON installation_keys(tenant_id);
CREATE INDEX idx_installation_keys_key_hash ON installation_keys(key_hash);
CREATE INDEX idx_installation_keys_expires_at ON installation_keys(expires_at);

-- Agent indices
CREATE INDEX idx_agents_tenant_id ON agents(tenant_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_tenant_status ON agents(tenant_id, status);
CREATE INDEX idx_agents_last_seen ON agents(last_seen_at);
CREATE INDEX idx_agents_hostname ON agents(hostname);

-- Agent tokens indices
CREATE INDEX idx_agent_tokens_agent_id ON agent_tokens(agent_id);
CREATE INDEX idx_agent_tokens_tenant_id ON agent_tokens(tenant_id);
CREATE INDEX idx_agent_tokens_token_hash ON agent_tokens(token_hash);

-- Workflow indices
CREATE INDEX idx_workflows_tenant_id ON workflows(tenant_id);
CREATE INDEX idx_workflows_status ON workflows(status);
CREATE INDEX idx_workflows_tenant_status ON workflows(tenant_id, status);
CREATE INDEX idx_workflows_name ON workflows(name);

-- Workflow execution indices
CREATE INDEX idx_workflow_executions_workflow_id ON workflow_executions(workflow_id);
CREATE INDEX idx_workflow_executions_tenant_id ON workflow_executions(tenant_id);
CREATE INDEX idx_workflow_executions_agent_id ON workflow_executions(agent_id);
CREATE INDEX idx_workflow_executions_campaign_id ON workflow_executions(campaign_id);
CREATE INDEX idx_workflow_executions_status ON workflow_executions(status);
CREATE INDEX idx_workflow_executions_tenant_status ON workflow_executions(tenant_id, status);
CREATE INDEX idx_workflow_executions_created_at ON workflow_executions(created_at);

-- Campaign indices
CREATE INDEX idx_campaigns_tenant_id ON campaigns(tenant_id);
CREATE INDEX idx_campaigns_workflow_id ON campaigns(workflow_id);
CREATE INDEX idx_campaigns_status ON campaigns(status);
CREATE INDEX idx_campaigns_tenant_status ON campaigns(tenant_id, status);

-- Campaign phases indices
CREATE INDEX idx_campaign_phases_campaign_id ON campaign_phases(campaign_id);
CREATE INDEX idx_campaign_phases_status ON campaign_phases(status);

-- Agent health reports indices
CREATE INDEX idx_agent_health_reports_agent_id ON agent_health_reports(agent_id);
CREATE INDEX idx_agent_health_reports_tenant_id ON agent_health_reports(tenant_id);
CREATE INDEX idx_agent_health_reports_reported_at ON agent_health_reports(reported_at);
CREATE INDEX idx_agent_health_reports_status ON agent_health_reports(status);

-- Composite indices for common queries
CREATE INDEX idx_agents_tenant_hostname ON agents(tenant_id, hostname);
CREATE INDEX idx_workflows_tenant_name ON workflows(tenant_id, name);
CREATE INDEX idx_workflow_executions_agent_status ON workflow_executions(agent_id, status);
