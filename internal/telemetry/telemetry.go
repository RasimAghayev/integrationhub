// Package telemetry configures OpenTelemetry tracing for the service.
package telemetry

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultServiceName = "integrationhub"
	defaultSampleRatio = 1.0
)

// Config holds the tracing configuration resolved from the environment.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
	Insecure       bool
	SampleRatio    float64
}

// FromEnv builds a Config from INTEGRATIONHUB_OTEL_* variables.
func FromEnv() Config {
	return Config{
		ServiceName:    envString("INTEGRATIONHUB_OTEL_SERVICE_NAME", defaultServiceName),
		ServiceVersion: os.Getenv("INTEGRATIONHUB_VERSION"),
		Endpoint:       os.Getenv("INTEGRATIONHUB_OTEL_ENDPOINT"),
		Insecure:       envBool("INTEGRATIONHUB_OTEL_INSECURE", true),
		SampleRatio:    envFloat("INTEGRATIONHUB_OTEL_SAMPLE_RATIO", defaultSampleRatio),
	}
}

// Enabled reports whether an OTLP endpoint is configured.
func (c Config) Enabled() bool { return c.Endpoint != "" }

// Init installs a global tracer provider and propagator. It returns a shutdown
// function that flushes pending spans; the caller must invoke it on exit.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled() {
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		cfg.SampleRatio = defaultSampleRatio
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.ServiceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler(cfg.SampleRatio)),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

func sampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 {
		return sdktrace.NeverSample()
	}
	if ratio >= 1 {
		return sdktrace.AlwaysSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

// Tracer returns a tracer for the given instrumentation scope.
func Tracer(name string) trace.Tracer { return otel.Tracer(name) }

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}

func envFloat(key string, def float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil {
		return def
	}
	return v
}

// Shutdown flushes and stops the tracer provider.
func Shutdown(ctx context.Context, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = fn(timeout)
}
