package obs

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Start sets up observability features such as tracing, metrics, and logging for the application.
//
// It initializes the OpenTelemetry SDK and configures different exporters and resource detectors.
// In addition to setting up the OpenTelemetry logging SDK, it bridges [log] and [slog] to [go.opentelemetry.io/otel/log] as well.
// Returns a shutdown function to close exporters and free resources, and an error if setup fails.
//
// Customize the exporters as specified in [autoexport].
func Start(ctx context.Context, name string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	var shutdownFuncs []func(context.Context) error

	shutdown := func(ctx context.Context) error {
		var err error
		for _, shutdownFunc := range shutdownFuncs {
			err = errors.Join(err, shutdownFunc(ctx))
		}
		return err
	}

	r, err := resource.New(ctx,
		resource.WithContainer(),
		resource.WithProcess(),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
		resource.WithDetectors(
			ecs.NewResourceDetector(),
		),
	)

	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, spanExporter.Shutdown)

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, metricReader.Shutdown)

	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, logExporter.Shutdown)

	var env string

	for i := r.Iter(); i.Next(); {
		attr := i.Attribute()
		if attr.Key == "deployment.environment.name" {
			env = attr.Value.AsString()
		}
	}

	var tracerProvider *trace.TracerProvider
	var meterProvider *metric.MeterProvider
	var logProvider *log.LoggerProvider

	var spanProcessor func(exporter trace.SpanExporter) trace.TracerProviderOption
	var logProcessor func(exporter log.Exporter) log.Processor

	if env == "local" {
		spanProcessor = trace.WithSyncer
		logProcessor = func(exporter log.Exporter) log.Processor { return log.NewSimpleProcessor(logExporter) }
	} else {
		spanProcessor = func(exporter trace.SpanExporter) trace.TracerProviderOption { return trace.WithBatcher(exporter) }
		logProcessor = func(exporter log.Exporter) log.Processor { return log.NewBatchProcessor(logExporter) }
	}

	tracerProvider = trace.NewTracerProvider(spanProcessor(spanExporter), trace.WithResource(r))
	meterProvider = metric.NewMeterProvider(metric.WithReader(metricReader), metric.WithResource(r))
	logProvider = log.NewLoggerProvider(log.WithProcessor(logProcessor(logExporter)), log.WithResource(r))

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	global.SetLoggerProvider(logProvider)

	slog.SetDefault(otelslog.NewLogger(name))

	return shutdown, nil
}
