package domain

import (
	"strings"
	"time"
)

// Schedule triggers an integration endpoint on a cron expression.
type Schedule struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	IntegrationID string     `json:"integration_id"`
	Endpoint      string     `json:"endpoint"`
	Cron          string     `json:"cron"`
	Enabled       bool       `json:"enabled"`
	NextRun       time.Time  `json:"next_run"`
	LastRun       *time.Time `json:"last_run,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Validate checks the required fields of a schedule.
func (s *Schedule) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return &ValidationError{"name", "is required"}
	}
	if strings.TrimSpace(s.IntegrationID) == "" {
		return &ValidationError{"integration_id", "is required"}
	}
	if strings.TrimSpace(s.Endpoint) == "" {
		return &ValidationError{"endpoint", "is required"}
	}
	if strings.TrimSpace(s.Cron) == "" {
		return &ValidationError{"cron", "is required"}
	}
	return nil
}
