// Package workflow provides workflow management for the control plane.
package workflow

import (
	"fmt"
	"strings"
)

// Validator validates workflow definitions
type Validator struct{}

// NewValidator creates a new workflow validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate validates a workflow definition
func (v *Validator) Validate(definition map[string]interface{}) error {
	var errors ValidationErrors

	// Check required fields
	if _, ok := definition["name"]; !ok {
		errors = append(errors, ValidationError{"name", "required field"})
	}

	// Check steps
	steps, ok := definition["steps"]
	if !ok {
		errors = append(errors, ValidationError{"steps", "required field"})
	} else {
		stepsList, ok := steps.([]interface{})
		if !ok {
			errors = append(errors, ValidationError{"steps", "must be an array"})
		} else if len(stepsList) == 0 {
			errors = append(errors, ValidationError{"steps", "must have at least one step"})
		} else {
			for i, step := range stepsList {
				if err := v.validateStep(i, step); err != nil {
					errors = append(errors, err...)
				}
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// validateStep validates a single step
func (v *Validator) validateStep(index int, step interface{}) ValidationErrors {
	var errors ValidationErrors
	prefix := fmt.Sprintf("steps[%d]", index)

	stepMap, ok := step.(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{prefix, "must be an object"})
		return errors
	}

	// Check step ID
	if _, ok := stepMap["id"]; !ok {
		errors = append(errors, ValidationError{prefix + ".id", "required field"})
	}

	// Check step name
	if _, ok := stepMap["name"]; !ok {
		errors = append(errors, ValidationError{prefix + ".name", "required field"})
	}

	// Check step type
	stepType, ok := stepMap["type"]
	if !ok {
		errors = append(errors, ValidationError{prefix + ".type", "required field"})
	} else {
		typeStr, ok := stepType.(string)
		if !ok {
			errors = append(errors, ValidationError{prefix + ".type", "must be a string"})
		} else {
			validTypes := map[string]bool{
				"command":  true,
				"script":   true,
				"file":     true,
				"http":     true,
				"validate": true,
			}
			if !validTypes[typeStr] {
				errors = append(errors, ValidationError{prefix + ".type", fmt.Sprintf("invalid type: %s", typeStr)})
			}
		}
	}

	// Check step has command or script based on type
	if stepType == "command" {
		if _, ok := stepMap["command"]; !ok {
			if _, ok := stepMap["args"]; !ok {
				errors = append(errors, ValidationError{prefix, "command or args required for command step"})
			}
		}
	}

	if stepType == "script" {
		if _, ok := stepMap["script"]; !ok {
			errors = append(errors, ValidationError{prefix + ".script", "required for script step"})
		}
	}

	// Validate timeout if present
	if timeout, ok := stepMap["timeout"]; ok {
		if _, ok := timeout.(string); !ok {
			errors = append(errors, ValidationError{prefix + ".timeout", "must be a string duration"})
		}
	}

	// Validate retry_count if present
	if retryCount, ok := stepMap["retry_count"]; ok {
		switch v := retryCount.(type) {
		case float64:
			if v < 0 {
				errors = append(errors, ValidationError{prefix + ".retry_count", "must be non-negative"})
			}
		case int:
			if v < 0 {
				errors = append(errors, ValidationError{prefix + ".retry_count", "must be non-negative"})
			}
		default:
			errors = append(errors, ValidationError{prefix + ".retry_count", "must be a number"})
		}
	}

	return errors
}

// ValidateForExecution validates a workflow is ready for execution
func (v *Validator) ValidateForExecution(definition map[string]interface{}) error {
	// First run standard validation
	if err := v.Validate(definition); err != nil {
		return err
	}

	// Additional checks for execution
	// For example, ensure all referenced files exist, etc.

	return nil
}
