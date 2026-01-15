// Package probe provides workflow execution functionality.
package probe

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// TemplateRenderer renders templates using Pongo2 (Jinja2-compatible)
type TemplateRenderer struct {
	// Additional template directories for includes/extends
	templateDirs []string
	// Custom filters
	customFilters map[string]pongo2.FilterFunction
	// Custom tags
	customTags map[string]pongo2.TagParser
}

// NewTemplateRenderer creates a new template renderer
func NewTemplateRenderer() *TemplateRenderer {
	r := &TemplateRenderer{
		templateDirs:  make([]string, 0),
		customFilters: make(map[string]pongo2.FilterFunction),
		customTags:    make(map[string]pongo2.TagParser),
	}

	// Register built-in custom filters useful for config management
	r.registerBuiltinFilters()

	return r
}

// registerBuiltinFilters registers useful custom filters for config management
func (r *TemplateRenderer) registerBuiltinFilters() {
	// default filter - returns default value if variable is empty/nil
	r.customFilters["default"] = func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if in.IsNil() || (in.IsString() && in.String() == "") {
			return param, nil
		}
		return in, nil
	}

	// quote filter - wraps string in quotes
	r.customFilters["quote"] = func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		return pongo2.AsValue(fmt.Sprintf("%q", in.String())), nil
	}

	// indent filter - indents each line by specified spaces
	r.customFilters["indent"] = func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		spaces := param.Integer()
		if spaces <= 0 {
			spaces = 4
		}
		indent := strings.Repeat(" ", spaces)
		lines := strings.Split(in.String(), "\n")
		for i, line := range lines {
			if line != "" {
				lines[i] = indent + line
			}
		}
		return pongo2.AsValue(strings.Join(lines, "\n")), nil
	}

	// bool filter - converts to boolean string (true/false)
	r.customFilters["bool"] = func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if in.Bool() {
			return pongo2.AsValue("true"), nil
		}
		return pongo2.AsValue("false"), nil
	}

	// yaml_encode filter - simple YAML encoding for values
	r.customFilters["yaml_encode"] = func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		// Simple implementation for common types
		if in.IsNil() {
			return pongo2.AsValue("null"), nil
		}
		if in.IsBool() {
			return pongo2.AsValue(fmt.Sprintf("%t", in.Bool())), nil
		}
		if in.IsInteger() {
			return pongo2.AsValue(fmt.Sprintf("%d", in.Integer())), nil
		}
		if in.IsFloat() {
			return pongo2.AsValue(fmt.Sprintf("%g", in.Float())), nil
		}
		// For strings, check if quoting is needed
		s := in.String()
		if strings.ContainsAny(s, ":#{}[]&*?|>!%@`") || s == "" {
			return pongo2.AsValue(fmt.Sprintf("%q", s)), nil
		}
		return in, nil
	}

	// Register all custom filters
	for name, fn := range r.customFilters {
		pongo2.RegisterFilter(name, fn)
	}
}

// RenderContext contains variables and metadata for template rendering
type RenderContext struct {
	// Vars contains the workflow variables (like Salt Pillar data)
	Vars map[string]interface{}
	// Env contains environment variables
	Env map[string]string
	// Facts contains system facts gathered from the agent
	Facts map[string]interface{}
}

// NewRenderContext creates a new render context with defaults
func NewRenderContext() *RenderContext {
	return &RenderContext{
		Vars:  make(map[string]interface{}),
		Env:   make(map[string]string),
		Facts: make(map[string]interface{}),
	}
}

// WithVars adds variables to the context
func (c *RenderContext) WithVars(vars map[string]interface{}) *RenderContext {
	for k, v := range vars {
		c.Vars[k] = v
	}
	return c
}

// WithEnv adds environment variables to the context
func (c *RenderContext) WithEnv(env map[string]string) *RenderContext {
	for k, v := range env {
		c.Env[k] = v
	}
	return c
}

// WithSystemFacts adds system facts to the context
func (c *RenderContext) WithSystemFacts() *RenderContext {
	// Gather basic system facts
	c.Facts["os"] = runtime.GOOS
	c.Facts["arch"] = runtime.GOARCH
	c.Facts["num_cpu"] = runtime.NumCPU()

	// Get hostname
	if hostname, err := os.Hostname(); err == nil {
		c.Facts["hostname"] = hostname
	}

	// Get home directory
	if home, err := os.UserHomeDir(); err == nil {
		c.Facts["home_dir"] = home
	}

	// Get current working directory
	if cwd, err := os.Getwd(); err == nil {
		c.Facts["cwd"] = cwd
	}

	return c
}

// ToContext converts RenderContext to pongo2.Context
func (c *RenderContext) ToContext() pongo2.Context {
	ctx := pongo2.Context{}

	// Add all vars directly to context (like Salt Pillar)
	for k, v := range c.Vars {
		ctx[k] = v
	}

	// Add env as a nested object
	ctx["env"] = c.Env

	// Add facts as a nested object
	ctx["facts"] = c.Facts

	return ctx
}

// RenderResult contains the result of template rendering
type RenderResult struct {
	// Content is the rendered content
	Content string
	// UsedVariables is a list of variables that were used in the template
	UsedVariables []string
}

// Render renders a template string with the given context
func (r *TemplateRenderer) Render(templateContent string, ctx *RenderContext) (*RenderResult, error) {
	// Parse the template
	tpl, err := pongo2.FromString(templateContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute the template
	output, err := tpl.Execute(ctx.ToContext())
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	return &RenderResult{
		Content: output,
	}, nil
}

// RenderString renders a simple string with variable interpolation
// This is useful for rendering destination paths and other simple strings
func (r *TemplateRenderer) RenderString(s string, ctx *RenderContext) (string, error) {
	// Skip if no template syntax detected
	if !strings.Contains(s, "{{") && !strings.Contains(s, "{%") {
		return s, nil
	}

	result, err := r.Render(s, ctx)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// RenderFile renders a template file with the given context
func (r *TemplateRenderer) RenderFile(templatePath string, ctx *RenderContext) (*RenderResult, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	return r.Render(string(content), ctx)
}

// AddTemplateDir adds a directory to search for template includes
func (r *TemplateRenderer) AddTemplateDir(dir string) {
	absDir, err := filepath.Abs(dir)
	if err == nil {
		r.templateDirs = append(r.templateDirs, absDir)
	}
}

// ValidateTemplate validates a template without rendering it
func (r *TemplateRenderer) ValidateTemplate(templateContent string) error {
	_, err := pongo2.FromString(templateContent)
	if err != nil {
		return fmt.Errorf("template validation failed: %w", err)
	}
	return nil
}
