package obs

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Start sets up OpenTelemetry tracing, metrics, and logging.
//
// After calling Start, you can use the OpenTelemetry API and instrument your code.
// The standard library [log] and [slog] packages are configured to send logs to OpenTelemetry.
// You can customize the exporters using environment variables as specified in [autoexport].
//
// Returns a shutdown function to close exporters and free resources, and an error if setup fails.
func Start(ctx context.Context, name string, opts ...resource.Option) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	var shutdownFuncs []func(context.Context) error

	shutdown := func(ctx context.Context) error {
		var err error
		for _, shutdownFunc := range shutdownFuncs {
			err = errors.Join(err, shutdownFunc(ctx))
		}
		return err
	}

	ropts := []resource.Option{
		resource.WithContainer(),
		resource.WithProcess(),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
	}
	ropts = append(ropts, opts...)

	r, err := resource.New(ctx, ropts...)

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
