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
	return nil
}
