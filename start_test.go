package obs_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/wolverian/obs"
)

func TestStart(t *testing.T) {
	// Set the environment to "none" so Start configures OpenTelemetry to export to the console.
	// We would otherwise get an exporter error from shutdown if there is no collector listening on localhost.
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=local")
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_LOGS_EXPORTER", "none")

	// Test basic initialisation
	ctx := context.Background()
	shutdown, err := obs.Start(ctx, "test-app", resource.WithAttributes(attribute.String("testkey", "foobar123")))

	if err != nil {
		t.Fatalf("Start returned an error: %v", err)
	}

	if shutdown == nil {
		t.Fatal("Start returned a nil shutdown function")
	}

	// Verify that global providers were set
	if otel.GetTracerProvider() == nil {
		t.Error("Tracer provider was not set")
	}

	if otel.GetMeterProvider() == nil {
		t.Error("Meter provider was not set")
	}

	if global.GetLoggerProvider() == nil {
		t.Error("Logger provider was not set")
	}

	// Verify resource attributes
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(t.Context(), "test-span")
	defer span.End()

	if !slices.ContainsFunc(span.(trace.ReadOnlySpan).Resource().Attributes(), func(kv attribute.KeyValue) bool {
		return kv.Key == "testkey" && kv.Value.AsString() == "foobar123"
	}) {
		t.Error("Resource attributes were not set")
	}

	// Call the shutdown function
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := shutdown(shutdownCtx); err != nil {
		t.Errorf("Shutdown returned an error: %v", err)
	}
}

func TestStartWithCanceledContext(t *testing.T) {
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	_, err := obs.Start(ctx, "test-app")
	if err == nil {
		t.Fatal("Start did not return an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Start returned an unexpected error: %v", err)
	}
}
