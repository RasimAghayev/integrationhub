package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	cfg := FromEnv()
	if cfg.ServiceName != defaultServiceName {
		t.Fatalf("service name %q", cfg.ServiceName)
	}
	if cfg.SampleRatio != 1 {
		t.Fatalf("sample ratio %v", cfg.SampleRatio)
	}
	if cfg.Enabled() {
		t.Fatal("tracing must be disabled without an endpoint")
	}
}

func TestInitDisabledNoError(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()
}

func TestInitInvalidRatioFallsBack(t *testing.T) {
	if got := sampler(-1); got == nil {
		t.Fatal("sampler must not be nil")
	}
	if got := sampler(0.5); got == nil {
		t.Fatal("sampler must not be nil")
	}
}

func TestShutdownNil(t *testing.T) {
	done := make(chan struct{})
	go func() {
		Shutdown(context.Background(), nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Shutdown(nil) must return immediately")
	}
}
