// Package events publishes integration lifecycle events to a message broker.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// Subject is the NATS subject prefix for all platform events.
const Subject = "integrationhub.events"

// Event is published whenever an execution reaches a terminal state.
type Event struct {
	Type           string `json:"type"`
	ExecutionID    string `json:"execution_id"`
	IntegrationID  string `json:"integration_id"`
	Endpoint       string `json:"endpoint"`
	Status         string `json:"status"`
	StatusCode     int    `json:"status_code"`
	Attempts       int    `json:"attempts"`
	Error          string `json:"error,omitempty"`
	ResponseLength int    `json:"response_length"`
}

// Publisher sends events to a broker.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// NATS publishes events to a NATS server.
type NATS struct {
	conn *nats.Conn
}

// NewNATS connects to url. A short timeout keeps startup bounded.
func NewNATS(url string) (*NATS, error) {
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(true))
	if err != nil {
		return nil, fmt.Errorf("events: connect: %w", err)
	}
	return &NATS{conn: nc}, nil
}

// Publish encodes the event and sends it to the subject.
func (n *NATS) Publish(_ context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	if err := n.conn.Publish(Subject+"."+event.Type, payload); err != nil {
		return fmt.Errorf("events: publish: %w", err)
	}
	return nil
}

// Close flushes and closes the connection.
func (n *NATS) Close() {
	if n == nil || n.conn == nil {
		return
	}
	_ = n.conn.Flush()
	n.conn.Close()
}

// Log is a Publisher that only logs; used when no broker is configured.
type Log struct{}

// Publish logs the event at debug level.
func (Log) Publish(_ context.Context, event Event) error {
	slog.Debug("event", "type", event.Type, "execution_id", event.ExecutionID, "status", event.Status)
	return nil
}
