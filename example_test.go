package obs_test

import (
	"context"
	"log"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/wolverian/obs"
)

func Example() {
	shutdown, err := obs.Start(context.Background(),
		"test-app",
		// Customise your resource by e.g. using detectors for your platform
		resource.WithDetectors(ecs.NewResourceDetector()),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelFunc()
		if err := shutdown(ctx); err != nil {
			log.Printf("error shutting down observability: %v", err)
		}
	}()

	Run(context.Background())
	// Output:
}

func Run(ctx context.Context) {
	slog.Info("Logs to OpenTelemetry")

	tracer := otel.Tracer("test-app")
	ctx, span := tracer.Start(ctx, "test-span")
	defer span.End()

	span.SetAttributes(attribute.String("example", "attribute"))

	// Do much work here
}
