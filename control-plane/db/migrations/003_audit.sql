-- Audit tables for Multi-Tenant VM Manager
-- Note: Primary audit logging is in Quickwit. These tables provide backup and quick queries.

-- Local audit log table (subset of full audit data)
CREATE TABLE IF NOT EXISTS audit_logs (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    actor_type ENUM('user', 'agent', 'system', 'api') NOT NULL DEFAULT 'user',
    resource_type VARCHAR(64),
    resource_id VARCHAR(64),
    action VARCHAR(64) NOT NULL,
    result ENUM('success', 'failure', 'error') NOT NULL,
    details JSON,
    ip_address VARCHAR(45),
    user_agent VARCHAR(512),
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_audit_tenant_id (tenant_id),
    INDEX idx_audit_event_type (event_type),
    INDEX idx_audit_actor (actor),
    INDEX idx_audit_resource (resource_type, resource_id),
    INDEX idx_audit_timestamp (timestamp),
    INDEX idx_audit_tenant_timestamp (tenant_id, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY RANGE (UNIX_TIMESTAMP(timestamp)) (
    PARTITION p_2024_01 VALUES LESS THAN (UNIX_TIMESTAMP('2024-02-01')),
    PARTITION p_2024_02 VALUES LESS THAN (UNIX_TIMESTAMP('2024-03-01')),
    PARTITION p_2024_03 VALUES LESS THAN (UNIX_TIMESTAMP('2024-04-01')),
    PARTITION p_2024_04 VALUES LESS THAN (UNIX_TIMESTAMP('2024-05-01')),
    PARTITION p_2024_05 VALUES LESS THAN (UNIX_TIMESTAMP('2024-06-01')),
    PARTITION p_2024_06 VALUES LESS THAN (UNIX_TIMESTAMP('2024-07-01')),
    PARTITION p_2024_07 VALUES LESS THAN (UNIX_TIMESTAMP('2024-08-01')),
    PARTITION p_2024_08 VALUES LESS THAN (UNIX_TIMESTAMP('2024-09-01')),
    PARTITION p_2024_09 VALUES LESS THAN (UNIX_TIMESTAMP('2024-10-01')),
    PARTITION p_2024_10 VALUES LESS THAN (UNIX_TIMESTAMP('2024-11-01')),
    PARTITION p_2024_11 VALUES LESS THAN (UNIX_TIMESTAMP('2024-12-01')),
    PARTITION p_2024_12 VALUES LESS THAN (UNIX_TIMESTAMP('2025-01-01')),
    PARTITION p_future VALUES LESS THAN MAXVALUE
);

-- Audit event types reference
CREATE TABLE IF NOT EXISTS audit_event_types (
    type_name VARCHAR(64) PRIMARY KEY,
    description TEXT,
    severity ENUM('low', 'medium', 'high', 'critical') NOT NULL DEFAULT 'low',
    retention_days INT DEFAULT 90,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Insert standard audit event types
INSERT INTO audit_event_types (type_name, description, severity) VALUES
    ('tenant.created', 'Tenant created', 'medium'),
    ('tenant.updated', 'Tenant updated', 'low'),
    ('tenant.deleted', 'Tenant deleted', 'high'),
    ('tenant.suspended', 'Tenant suspended', 'high'),
    ('agent.registered', 'Agent registered', 'medium'),
    ('agent.deregistered', 'Agent deregistered', 'medium'),
    ('agent.online', 'Agent came online', 'low'),
    ('agent.offline', 'Agent went offline', 'low'),
    ('agent.upgraded', 'Agent upgraded', 'medium'),
    ('workflow.created', 'Workflow created', 'low'),
    ('workflow.updated', 'Workflow updated', 'low'),
    ('workflow.deleted', 'Workflow deleted', 'medium'),
    ('workflow.executed', 'Workflow executed', 'low'),
    ('workflow.completed', 'Workflow completed', 'low'),
    ('workflow.failed', 'Workflow failed', 'medium'),
    ('campaign.created', 'Campaign created', 'medium'),
    ('campaign.started', 'Campaign started', 'medium'),
    ('campaign.completed', 'Campaign completed', 'medium'),
    ('campaign.failed', 'Campaign failed', 'high'),
    ('campaign.cancelled', 'Campaign cancelled', 'medium'),
    ('campaign.rollback', 'Campaign rollback initiated', 'high'),
    ('auth.login', 'User login', 'low'),
    ('auth.logout', 'User logout', 'low'),
    ('auth.failed', 'Authentication failed', 'medium'),
    ('key.created', 'API/Installation key created', 'medium'),
    ('key.revoked', 'API/Installation key revoked', 'medium'),
    ('key.used', 'Installation key used', 'low'),
    ('config.changed', 'Configuration changed', 'medium'),
    ('security.alert', 'Security alert', 'critical')
ON DUPLICATE KEY UPDATE description = VALUES(description);

-- Stored procedure for audit log cleanup
DELIMITER //
CREATE PROCEDURE IF NOT EXISTS cleanup_audit_logs(IN days_to_keep INT)
BEGIN
    DECLARE cutoff_timestamp TIMESTAMP;
    SET cutoff_timestamp = DATE_SUB(NOW(), INTERVAL days_to_keep DAY);

    DELETE FROM audit_logs WHERE timestamp < cutoff_timestamp;

    SELECT ROW_COUNT() as deleted_rows;
END //
DELIMITER ;

-- View for recent audit activity
CREATE OR REPLACE VIEW v_recent_audit_activity AS
SELECT
    a.id,
    a.tenant_id,
    t.name as tenant_name,
    a.event_type,
    a.actor,
    a.actor_type,
    a.resource_type,
    a.resource_id,
    a.action,
    a.result,
    a.timestamp
FROM audit_logs a
LEFT JOIN tenants t ON a.tenant_id = t.id
WHERE a.timestamp > DATE_SUB(NOW(), INTERVAL 24 HOUR)
ORDER BY a.timestamp DESC;
