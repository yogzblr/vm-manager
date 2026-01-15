-- Templates schema for Salt Stack-like configuration management
-- MySQL 8.0+

-- Templates table (stores Jinja2-compatible templates)
CREATE TABLE IF NOT EXISTS templates (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    content LONGTEXT NOT NULL,
    content_type VARCHAR(100) DEFAULT 'text/plain',
    version INT NOT NULL DEFAULT 1,
    status ENUM('draft', 'active', 'deprecated', 'deleted') NOT NULL DEFAULT 'draft',
    tags JSON,
    metadata JSON,
    created_by VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE KEY idx_templates_tenant_name (tenant_id, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Template versions table (tracks version history)
CREATE TABLE IF NOT EXISTS template_versions (
    id VARCHAR(64) PRIMARY KEY,
    template_id VARCHAR(64) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    version INT NOT NULL,
    content LONGTEXT NOT NULL,
    changed_by VARCHAR(255),
    change_note TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE KEY idx_template_versions_version (template_id, version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Index for efficient template lookups
CREATE INDEX idx_templates_status ON templates(status);
CREATE INDEX idx_templates_tenant_status ON templates(tenant_id, status);
CREATE INDEX idx_template_versions_template ON template_versions(template_id);
