package executor

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the executor's Prometheus collectors.
type Metrics struct {
	Requests *prometheus.CounterVec
	Latency  *prometheus.HistogramVec
	Breakers *prometheus.GaugeVec
}

// NewMetrics builds the executor collectors.
func NewMetrics() *Metrics {
	return &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "integrationhub_executor_requests_total",
			Help: "Total endpoint executions by integration and outcome.",
		}, []string{"integration", "outcome"}),
		Latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "integrationhub_executor_latency_ms",
			Help:    "Execution latency in milliseconds by integration.",
			Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
		}, []string{"integration"}),
		Breakers: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "integrationhub_executor_breaker_open",
			Help: "1 when the connection circuit breaker is open.",
		}, []string{"connection"}),
	}
}

// Collectors returns the collectors to register with a registry.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.Requests, m.Latency, m.Breakers}
}
