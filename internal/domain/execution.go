package domain

import "time"

// ExecutionStatus is the lifecycle state of a single endpoint invocation.
type ExecutionStatus string

const (
	ExecutionQueued    ExecutionStatus = "queued"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionSucceeded ExecutionStatus = "succeeded"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionDead      ExecutionStatus = "dead"
)

// ValidExecutionStatuses is the set of accepted execution statuses.
var ValidExecutionStatuses = map[ExecutionStatus]bool{
	ExecutionQueued:    true,
	ExecutionRunning:   true,
	ExecutionSucceeded: true,
	ExecutionFailed:    true,
	ExecutionDead:      true,
}

// Execution is a persisted record of one endpoint call, including its outcome.
// Records in the dead state form the dead-letter queue and can be replayed.
type Execution struct {
	ID            string          `json:"id"`
	IntegrationID string          `json:"integration_id"`
	Endpoint      string          `json:"endpoint"`
	Status        ExecutionStatus `json:"status"`
	RequestBody   string          `json:"request_body,omitempty"`
	ResponseBody  string          `json:"response_body,omitempty"`
	StatusCode    int             `json:"status_code,omitempty"`
	Attempts      int             `json:"attempts"`
	Error         string          `json:"error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
}

// IsTerminal reports whether the execution has reached a final state.
func (e *Execution) IsTerminal() bool {
	return e.Status == ExecutionSucceeded || e.Status == ExecutionFailed || e.Status == ExecutionDead
}
