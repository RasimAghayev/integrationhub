package domain

import (
	"strconv"
	"strings"
	"time"
)

// IntegrationType enumerates the kind of upstream API an integration wraps.
type IntegrationType string

const (
	IntegrationREST    IntegrationType = "rest"
	IntegrationGraphQL IntegrationType = "graphql"
	IntegrationWebhook IntegrationType = "webhook"
)

// ValidIntegrationTypes is the set of accepted integration types.
var ValidIntegrationTypes = map[IntegrationType]bool{
	IntegrationREST:    true,
	IntegrationGraphQL: true,
	IntegrationWebhook: true,
}

// IntegrationStatus is the lifecycle state of an integration.
type IntegrationStatus string

const (
	StatusActive IntegrationStatus = "active"
	StatusPaused IntegrationStatus = "paused"
	StatusError  IntegrationStatus = "error"
)

// ValidIntegrationStatuses is the set of accepted statuses.
var ValidIntegrationStatuses = map[IntegrationStatus]bool{
	StatusActive: true,
	StatusPaused: true,
	StatusError:  true,
}

// Endpoint is a single callable operation of an integration.
type Endpoint struct {
	Name         string `json:"name"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	BodyTemplate string `json:"body_template,omitempty"`
}

// validTransforms is the set of accepted per-field transform names.
var validTransforms = map[string]bool{
	"": true, "none": true, "upper": true, "lower": true, "trim": true,
	"to_int": true, "to_string": true, "to_bool": true,
}

// ValidateTransform reports whether name is a supported per-field transform.
func ValidateTransform(name string) bool {
	if strings.HasPrefix(name, "default:") || strings.HasPrefix(name, "template:") {
		return true
	}
	return validTransforms[name]
}

// FieldMapping copies one source field of an upstream response to a target
// field of the normalized output, optionally applying a transform.
type FieldMapping struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Transform string `json:"transform,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

// TransformType enumerates the supported response transform kinds.
type TransformType string

const (
	TransformNone     TransformType = "none"
	TransformTemplate TransformType = "template"
	TransformLua      TransformType = "lua"
)

// ValidTransformTypes is the set of accepted transform kinds.
var ValidTransformTypes = map[TransformType]bool{
	TransformNone:     true,
	TransformTemplate: true,
	TransformLua:      true,
}

// TransformConfig post-processes the mapped payload.
type TransformConfig struct {
	Type     TransformType `json:"type"`
	Template string        `json:"template,omitempty"`
	Script   string        `json:"script,omitempty"`
}

// Integration is a configured connection to a 3rd-party API plus the endpoints
// to call and the metadata needed to execute them.
type Integration struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Type         IntegrationType   `json:"type"`
	Status       IntegrationStatus `json:"status"`
	ConnectionID string            `json:"connection_id"`
	Endpoints    []Endpoint        `json:"endpoints"`
	Mappings     []FieldMapping    `json:"mappings"`
	Transform    TransformConfig   `json:"transform"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Validate checks the required fields and enum values of an integration.
func (i *Integration) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return &ValidationError{"name", "is required"}
	}
	if !ValidIntegrationTypes[i.Type] {
		return &ValidationError{"type", "must be one of rest, graphql, webhook"}
	}
	if i.Status == "" {
		i.Status = StatusActive
	}
	if !ValidIntegrationStatuses[i.Status] {
		return &ValidationError{"status", "must be one of active, paused, error"}
	}
	if strings.TrimSpace(i.ConnectionID) == "" {
		return &ValidationError{"connection_id", "is required"}
	}
	for idx, e := range i.Endpoints {
		if strings.TrimSpace(e.Name) == "" {
			return &ValidationError{"endpoints[" + strconv.Itoa(idx) + "].name", "is required"}
		}
		if strings.TrimSpace(e.Method) == "" {
			return &ValidationError{"endpoints[" + strconv.Itoa(idx) + "].method", "is required"}
		}
		if strings.TrimSpace(e.Path) == "" {
			return &ValidationError{"endpoints[" + strconv.Itoa(idx) + "].path", "is required"}
		}
	}
	for idx, m := range i.Mappings {
		if strings.TrimSpace(m.Source) == "" {
			return &ValidationError{"mappings[" + strconv.Itoa(idx) + "].source", "is required"}
		}
		if strings.TrimSpace(m.Target) == "" {
			return &ValidationError{"mappings[" + strconv.Itoa(idx) + "].target", "is required"}
		}
		if !ValidateTransform(m.Transform) {
			return &ValidationError{"mappings[" + strconv.Itoa(idx) + "].transform", "unsupported transform"}
		}
	}
	if i.Transform.Type == "" {
		i.Transform.Type = TransformNone
	}
	if !ValidTransformTypes[i.Transform.Type] {
		return &ValidationError{"transform.type", "must be one of none, template, lua"}
	}
	if i.Transform.Type == TransformTemplate && strings.TrimSpace(i.Transform.Template) == "" {
		return &ValidationError{"transform.template", "is required for template transforms"}
	}
	if i.Transform.Type == TransformLua && strings.TrimSpace(i.Transform.Script) == "" {
		return &ValidationError{"transform.script", "is required for lua transforms"}
	}
	return nil
}
