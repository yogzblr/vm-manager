# Workflow Examples for VM Agent Probe

This document provides comprehensive examples of using the VM Agent probe system for configuration management and automation tasks, including the Salt Stack-like template deployment features.

## Table of Contents

- [Workflow Structure](#workflow-structure)
- [Step Types](#step-types)
- [Template Deployment](#template-deployment)
- [Service Configuration Examples](#service-configuration-examples)
- [Cross-Platform Workflows](#cross-platform-workflows)
- [Advanced Patterns](#advanced-patterns)

---

## Workflow Structure

A workflow is a YAML document that defines a series of steps to execute on target agents.

### Basic Structure

```yaml
id: "unique-workflow-id"
name: "Human Readable Name"
description: "What this workflow does"
version: "1.0.0"
timeout: "30m"

# Global environment variables (available to all steps)
env:
  APP_ENV: production
  LOG_LEVEL: info

# Template variables (like Salt Pillar data)
vars:
  domain: example.com
  port: 8080
  admin_email: admin@example.com

# Execution steps
steps:
  - id: step-1
    name: "First Step"
    type: command
    command: echo "Hello World"

# Lifecycle hooks
on_success:
  - id: notify-success
    type: command
    command: echo "Workflow completed successfully"

on_failure:
  - id: notify-failure
    type: command
    command: echo "Workflow failed"

on_cancel:
  - id: cleanup
    type: command
    command: echo "Workflow was cancelled"
```

### Step Configuration Options

Each step supports the following options:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier for the step (required) |
| `name` | string | Human-readable name |
| `type` | string | Step type: `command`, `script`, `template` |
| `command` | string | Shell command to execute |
| `script` | string | Inline script content |
| `template` | object | Template configuration (for type: template) |
| `env` | map | Step-specific environment variables |
| `work_dir` | string | Working directory for execution |
| `timeout` | duration | Step timeout (default: 5m) |
| `retry_count` | int | Number of retries on failure |
| `retry_delay` | duration | Delay between retries |
| `continue_on_error` | bool | Continue workflow if step fails |
| `condition` | string | Shell condition to evaluate before running |

---

## Step Types

### Command Step

Execute a shell command directly.

```yaml
steps:
  - id: check-disk
    name: "Check Disk Space"
    type: command
    command: df -h /
    timeout: 30s

  - id: install-package
    name: "Install nginx"
    type: command
    command: apt-get install -y nginx
    retry_count: 3
    retry_delay: 10s
```

### Script Step

Execute an inline multi-line script.

```yaml
steps:
  - id: setup-app
    name: "Setup Application"
    type: script
    script: |
      #!/bin/bash
      set -e

      echo "Creating directories..."
      mkdir -p /var/www/app
      mkdir -p /var/log/app

      echo "Setting permissions..."
      chown -R www-data:www-data /var/www/app
      chmod 755 /var/www/app

      echo "Setup complete!"
    timeout: 5m
```

### Template Step

Deploy configuration files using Jinja2-compatible templates.

```yaml
steps:
  - id: deploy-config
    name: "Deploy Application Config"
    type: template
    template:
      source: "https://templates.example.com/app-config.yaml.j2"
      dest: "/etc/myapp/config.yaml"
      mode: "0644"
      owner: "myapp"
      group: "myapp"
      backup: true
      create_dirs: true
```

---

## Template Deployment

The template system provides Salt Stack-like configuration management capabilities.

### Template Sources

Templates can be fetched from two sources:

1. **HTTP/HTTPS URLs**: Any accessible web endpoint
   ```yaml
   source: "https://templates.example.com/nginx.conf.j2"
   ```

2. **Control Plane**: Templates stored in the central control plane
   ```yaml
   source: "control-plane://templates/nginx-vhost"
   ```

### Template Syntax (Jinja2/Pongo2)

Templates use Jinja2-compatible syntax:

```jinja2
# Example: nginx-vhost.conf.j2
server {
    listen {{ port | default:80 }};
    server_name {{ domain }};

    root {{ document_root | default:"/var/www/html" }};

    {% if ssl_enabled %}
    listen 443 ssl;
    ssl_certificate /etc/ssl/certs/{{ domain }}.crt;
    ssl_certificate_key /etc/ssl/private/{{ domain }}.key;
    {% endif %}

    location / {
        try_files $uri $uri/ =404;
    }

    {% for location in custom_locations %}
    location {{ location.path }} {
        proxy_pass {{ location.backend }};
    }
    {% endfor %}

    access_log /var/log/nginx/{{ domain }}-access.log;
    error_log /var/log/nginx/{{ domain }}-error.log;
}
```

### Available Variables in Templates

| Variable | Description |
|----------|-------------|
| `vars.*` | All workflow variables (top-level access) |
| `env.*` | Environment variables |
| `facts.os` | Operating system (linux, windows, darwin) |
| `facts.arch` | Architecture (amd64, arm64) |
| `facts.hostname` | Agent hostname |
| `facts.num_cpu` | Number of CPUs |
| `facts.home_dir` | User home directory |

### Built-in Filters

| Filter | Description | Example |
|--------|-------------|---------|
| `default` | Default value if empty | `{{ port \| default:8080 }}` |
| `quote` | Wrap in quotes | `{{ path \| quote }}` |
| `indent` | Indent lines | `{{ content \| indent:4 }}` |
| `bool` | Convert to true/false | `{{ enabled \| bool }}` |
| `yaml_encode` | YAML-safe encoding | `{{ value \| yaml_encode }}` |

### Template Step Options

```yaml
template:
  # Required
  source: "https://example.com/template.j2"  # Template source
  dest: "/etc/app/config.yaml"               # Destination path

  # Optional
  mode: "0644"           # File permissions (Unix octal)
  owner: "appuser"       # File owner
  group: "appgroup"      # File group (Unix only)
  backup: true           # Backup existing file before overwrite
  diff_only: false       # Only show diff, don't write
  create_dirs: true      # Create parent directories
```

---

## Service Configuration Examples

### Example 1: Apache Virtual Host Deployment

Deploy an Apache virtual host configuration similar to Salt Stack.

```yaml
id: "apache-vhost-deployment"
name: "Deploy Apache Virtual Host"
version: "1.0.0"
timeout: "10m"

vars:
  domain: "myapp.example.com"
  document_root: "/var/www/myapp"
  admin_email: "webmaster@example.com"
  max_clients: 150
  enable_ssl: false

steps:
  # Step 1: Ensure Apache is installed
  - id: install-apache
    name: "Install Apache"
    type: command
    command: apt-get update && apt-get install -y apache2
    condition: "! which apache2"

  # Step 2: Create document root
  - id: create-docroot
    name: "Create Document Root"
    type: script
    script: |
      mkdir -p {{ document_root }}
      chown -R www-data:www-data {{ document_root }}
      chmod 755 {{ document_root }}

  # Step 3: Deploy virtual host configuration
  - id: deploy-vhost
    name: "Deploy Virtual Host Config"
    type: template
    template:
      source: "control-plane://templates/apache-vhost"
      dest: "/etc/apache2/sites-available/{{ domain }}.conf"
      mode: "0644"
      owner: "root"
      group: "root"
      backup: true

  # Step 4: Enable the site
  - id: enable-site
    name: "Enable Site"
    type: command
    command: a2ensite {{ domain }}.conf

  # Step 5: Test Apache configuration
  - id: test-config
    name: "Test Apache Config"
    type: command
    command: apache2ctl configtest

  # Step 6: Reload Apache
  - id: reload-apache
    name: "Reload Apache"
    type: command
    command: systemctl reload apache2

on_failure:
  - id: rollback-config
    name: "Rollback Configuration"
    type: command
    command: |
      if [ -f "/etc/apache2/sites-available/{{ domain }}.conf.bak" ]; then
        mv /etc/apache2/sites-available/{{ domain }}.conf.bak \
           /etc/apache2/sites-available/{{ domain }}.conf
        systemctl reload apache2
      fi
```

**Apache Virtual Host Template** (`apache-vhost.conf.j2`):

```jinja2
<VirtualHost *:80>
    ServerName {{ domain }}
    ServerAdmin {{ admin_email }}
    DocumentRoot {{ document_root }}

    <Directory {{ document_root }}>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    {% if max_clients %}
    MaxRequestWorkers {{ max_clients }}
    {% endif %}

    ErrorLog ${APACHE_LOG_DIR}/{{ domain }}-error.log
    CustomLog ${APACHE_LOG_DIR}/{{ domain }}-access.log combined

    {% if enable_ssl %}
    RewriteEngine On
    RewriteCond %{HTTPS} off
    RewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]
    {% endif %}
</VirtualHost>

{% if enable_ssl %}
<VirtualHost *:443>
    ServerName {{ domain }}
    ServerAdmin {{ admin_email }}
    DocumentRoot {{ document_root }}

    SSLEngine on
    SSLCertificateFile /etc/ssl/certs/{{ domain }}.crt
    SSLCertificateKeyFile /etc/ssl/private/{{ domain }}.key

    <Directory {{ document_root }}>
        Options -Indexes +FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>

    ErrorLog ${APACHE_LOG_DIR}/{{ domain }}-ssl-error.log
    CustomLog ${APACHE_LOG_DIR}/{{ domain }}-ssl-access.log combined
</VirtualHost>
{% endif %}
```

### Example 2: Nginx Configuration with Upstream

```yaml
id: "nginx-loadbalancer"
name: "Deploy Nginx Load Balancer"
version: "1.0.0"

vars:
  app_name: "myapi"
  domain: "api.example.com"
  upstream_servers:
    - host: "10.0.1.10"
      port: 8080
      weight: 5
    - host: "10.0.1.11"
      port: 8080
      weight: 3
    - host: "10.0.1.12"
      port: 8080
      weight: 2
  max_fails: 3
  fail_timeout: 30

steps:
  - id: deploy-upstream
    name: "Deploy Upstream Config"
    type: template
    template:
      source: "control-plane://templates/nginx-upstream"
      dest: "/etc/nginx/conf.d/upstream-{{ app_name }}.conf"
      mode: "0644"
      backup: true

  - id: deploy-server
    name: "Deploy Server Config"
    type: template
    template:
      source: "control-plane://templates/nginx-server"
      dest: "/etc/nginx/sites-available/{{ domain }}.conf"
      mode: "0644"
      backup: true

  - id: enable-site
    name: "Enable Site"
    type: command
    command: ln -sf /etc/nginx/sites-available/{{ domain }}.conf /etc/nginx/sites-enabled/

  - id: test-nginx
    name: "Test Nginx Config"
    type: command
    command: nginx -t

  - id: reload-nginx
    name: "Reload Nginx"
    type: command
    command: systemctl reload nginx
```

### Example 3: Application Configuration File

```yaml
id: "app-config-deploy"
name: "Deploy Application Configuration"
version: "1.0.0"

vars:
  app_name: "payment-service"
  environment: "production"
  database:
    host: "db.internal.example.com"
    port: 5432
    name: "payments"
    pool_size: 20
  redis:
    host: "redis.internal.example.com"
    port: 6379
  features:
    new_checkout: true
    legacy_api: false
  log_level: "info"

steps:
  - id: deploy-app-config
    name: "Deploy Application Config"
    type: template
    template:
      source: "control-plane://templates/app-config-yaml"
      dest: "/etc/{{ app_name }}/config.yaml"
      mode: "0640"
      owner: "{{ app_name }}"
      group: "{{ app_name }}"
      backup: true
      create_dirs: true

  - id: validate-config
    name: "Validate Configuration"
    type: command
    command: /opt/{{ app_name }}/bin/validate-config /etc/{{ app_name }}/config.yaml
    continue_on_error: false

  - id: restart-service
    name: "Restart Service"
    type: command
    command: systemctl restart {{ app_name }}

  - id: health-check
    name: "Health Check"
    type: command
    command: |
      for i in 1 2 3 4 5; do
        curl -sf http://localhost:8080/health && exit 0
        sleep 5
      done
      exit 1
    timeout: 60s
```

**Application Config Template** (`app-config.yaml.j2`):

```jinja2
# Configuration for {{ app_name }}
# Environment: {{ environment }}
# Generated by VM Agent

app:
  name: {{ app_name }}
  environment: {{ environment }}

database:
  host: {{ database.host }}
  port: {{ database.port | default:5432 }}
  name: {{ database.name }}
  pool_size: {{ database.pool_size | default:10 }}

redis:
  host: {{ redis.host }}
  port: {{ redis.port | default:6379 }}

logging:
  level: {{ log_level | default:"info" }}
  format: json

features:
{% for feature, enabled in features.items() %}
  {{ feature }}: {{ enabled | bool }}
{% endfor %}

# System facts
runtime:
  hostname: {{ facts.hostname }}
  os: {{ facts.os }}
  arch: {{ facts.arch }}
  cpus: {{ facts.num_cpu }}
```

---

## Cross-Platform Workflows

### File Permissions

File permissions work across platforms with automatic mapping:

| Unix Mode | Windows ACL |
|-----------|-------------|
| `0644` | Owner: Read+Write, Everyone: Read |
| `0755` | Owner: Full, Everyone: Read+Execute |
| `0600` | Owner: Read+Write, Everyone: None |
| `0400` | Owner: Read, Everyone: None |

### Cross-Platform Example

```yaml
id: "cross-platform-config"
name: "Deploy Config (Windows & Linux)"
version: "1.0.0"

vars:
  app_name: "myservice"
  config_content: |
    server.port=8080
    server.host=0.0.0.0

steps:
  # Linux deployment
  - id: deploy-linux
    name: "Deploy Config (Linux)"
    type: template
    template:
      source: "control-plane://templates/app-properties"
      dest: "/etc/{{ app_name }}/application.properties"
      mode: "0644"
      owner: "{{ app_name }}"
      backup: true
    condition: "test $(uname) = 'Linux'"

  # Windows deployment
  - id: deploy-windows
    name: "Deploy Config (Windows)"
    type: template
    template:
      source: "control-plane://templates/app-properties"
      dest: "C:\\ProgramData\\{{ app_name }}\\application.properties"
      mode: "0644"  # Maps to: Owner RW, Everyone R
      owner: "SYSTEM"
      backup: true
    condition: "test $(uname) = 'Windows_NT' 2>/dev/null || echo Windows"

  # Restart service (Linux)
  - id: restart-linux
    name: "Restart Service (Linux)"
    type: command
    command: systemctl restart {{ app_name }}
    condition: "test $(uname) = 'Linux'"

  # Restart service (Windows)
  - id: restart-windows
    name: "Restart Service (Windows)"
    type: command
    command: net stop {{ app_name }} && net start {{ app_name }}
    condition: "test $(uname) = 'Windows_NT' 2>/dev/null || echo Windows"
```

---

## Advanced Patterns

### Conditional Execution

```yaml
steps:
  # Only run if file doesn't exist
  - id: initial-setup
    name: "Initial Setup"
    type: script
    script: |
      echo "Running initial setup..."
      touch /var/lib/app/.initialized
    condition: "! test -f /var/lib/app/.initialized"

  # Only run on Debian-based systems
  - id: apt-install
    name: "Install via APT"
    type: command
    command: apt-get install -y package-name
    condition: "which apt-get"

  # Only run on RHEL-based systems
  - id: yum-install
    name: "Install via YUM"
    type: command
    command: yum install -y package-name
    condition: "which yum"
```

### Retry Pattern with Backoff

```yaml
steps:
  - id: download-artifact
    name: "Download Artifact"
    type: command
    command: curl -fSL https://releases.example.com/app-v1.0.tar.gz -o /tmp/app.tar.gz
    retry_count: 5
    retry_delay: 10s
    timeout: 5m

  - id: flaky-api-call
    name: "Call External API"
    type: command
    command: curl -X POST https://api.example.com/notify
    retry_count: 3
    retry_delay: 5s
    continue_on_error: true
```

### Multi-File Deployment

```yaml
id: "multi-file-deployment"
name: "Deploy Multiple Config Files"
version: "1.0.0"

vars:
  app_name: "webapp"
  configs:
    - name: "main"
      template: "main-config"
      dest: "/etc/{{ app_name }}/config.yaml"
    - name: "logging"
      template: "logging-config"
      dest: "/etc/{{ app_name }}/logging.yaml"
    - name: "secrets"
      template: "secrets-config"
      dest: "/etc/{{ app_name }}/secrets.yaml"
      mode: "0600"

steps:
  - id: deploy-main-config
    name: "Deploy Main Config"
    type: template
    template:
      source: "control-plane://templates/main-config"
      dest: "/etc/{{ app_name }}/config.yaml"
      mode: "0644"
      backup: true
      create_dirs: true

  - id: deploy-logging-config
    name: "Deploy Logging Config"
    type: template
    template:
      source: "control-plane://templates/logging-config"
      dest: "/etc/{{ app_name }}/logging.yaml"
      mode: "0644"
      backup: true

  - id: deploy-secrets-config
    name: "Deploy Secrets Config"
    type: template
    template:
      source: "control-plane://templates/secrets-config"
      dest: "/etc/{{ app_name }}/secrets.yaml"
      mode: "0600"
      backup: true

  - id: reload-service
    name: "Reload Service"
    type: command
    command: systemctl reload {{ app_name }}
```

### Rollback on Failure

```yaml
id: "safe-deployment"
name: "Safe Deployment with Rollback"
version: "1.0.0"

vars:
  service_name: "api-service"

steps:
  - id: backup-current
    name: "Backup Current Config"
    type: command
    command: cp -r /etc/{{ service_name }} /etc/{{ service_name }}.rollback

  - id: deploy-new-config
    name: "Deploy New Configuration"
    type: template
    template:
      source: "control-plane://templates/api-config"
      dest: "/etc/{{ service_name }}/config.yaml"
      mode: "0644"

  - id: test-config
    name: "Test Configuration"
    type: command
    command: /opt/{{ service_name }}/bin/test-config

  - id: restart-service
    name: "Restart Service"
    type: command
    command: systemctl restart {{ service_name }}

  - id: health-check
    name: "Health Check"
    type: command
    command: curl -sf http://localhost:8080/health
    timeout: 30s
    retry_count: 6
    retry_delay: 5s

on_failure:
  - id: rollback
    name: "Rollback Configuration"
    type: script
    script: |
      echo "Deployment failed, rolling back..."
      rm -rf /etc/{{ service_name }}
      mv /etc/{{ service_name }}.rollback /etc/{{ service_name }}
      systemctl restart {{ service_name }}
      echo "Rollback complete"

on_success:
  - id: cleanup-backup
    name: "Cleanup Rollback Backup"
    type: command
    command: rm -rf /etc/{{ service_name }}.rollback
```

---

## API Usage

### Execute Workflow via API

```bash
# Create a workflow
curl -X POST https://control-plane/api/v1/workflows \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Deploy Apache Config",
    "definition": {
      "vars": {"domain": "example.com"},
      "steps": [...]
    }
  }'

# Execute workflow on specific agents
curl -X POST https://control-plane/api/v1/campaigns \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_id": "workflow-uuid",
    "name": "Apache Rollout",
    "target_selector": {"tags": {"role": "webserver"}},
    "phase_config": {"batch_size": 10, "delay": "5m"}
  }'
```

### Template Management API

```bash
# Create a template
curl -X POST https://control-plane/api/v1/templates \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "apache-vhost",
    "description": "Apache Virtual Host Template",
    "content": "<VirtualHost>...</VirtualHost>",
    "tags": {"service": "apache", "type": "vhost"}
  }'

# Get template content (for agents)
curl https://control-plane/api/v1/templates/{id}/content \
  -H "Authorization: Bearer $TOKEN"
```

---

## Best Practices

1. **Always use backups**: Enable `backup: true` for production configs
2. **Test configs before applying**: Use validation commands
3. **Use health checks**: Verify services are healthy after changes
4. **Implement rollbacks**: Define `on_failure` hooks for critical deployments
5. **Version your templates**: Use template versioning in control plane
6. **Use meaningful IDs**: Step IDs should describe what they do
7. **Set appropriate timeouts**: Don't use default timeouts for long operations
8. **Use conditions wisely**: Skip unnecessary steps based on system state
