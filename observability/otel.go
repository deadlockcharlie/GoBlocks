package observability

import (
	"context"
	"errors"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const name = "goblocks"

var (
	Tracer = otel.Tracer(name)
	Meter  = otel.Meter(name)
	Logger = otelslog.NewLogger(name)
	// Counters
	PutCount            metric.Int64Counter
	GetCount            metric.Int64Counter
	DeleteCount         metric.Int64Counter
	HealthCount         metric.Int64Counter
	InternalPutCount    metric.Int64Counter
	InternalDeleteCount metric.Int64Counter
	ErrorCount          metric.Int64Counter
	// Histograms for latency
	PutDuration    metric.Float64Histogram
	GetDuration    metric.Float64Histogram
	DeleteDuration metric.Float64Histogram
	// Gauges
	BlocksStored metric.Int64UpDownCounter
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown calls cleanup function registered via shutdownFuncs.

	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	tracerProvider, err := newTracerProvider(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	// Initialize counters
	PutCount, _ = Meter.Int64Counter("blocks.put.count",
		metric.WithDescription("Total number of PUT operations"))
	GetCount, _ = Meter.Int64Counter("blocks.get.count",
		metric.WithDescription("Total number of GET operations"))
	DeleteCount, _ = Meter.Int64Counter("blocks.delete.count",
		metric.WithDescription("Total number of DELETE operations"))
	HealthCount, _ = Meter.Int64Counter("blocks.health.count",
		metric.WithDescription("Total number of health check requests"))
	InternalPutCount, _ = Meter.Int64Counter("blocks.internalput.count",
		metric.WithDescription("Total number of internal PUT operations"))
	InternalDeleteCount, _ = Meter.Int64Counter("blocks.internaldelete.count",
		metric.WithDescription("Total number of internal DELETE operations"))
	ErrorCount, _ = Meter.Int64Counter("blocks.errors.count",
		metric.WithDescription("Total number of errors"))

	// Initialize histograms for latency (in milliseconds)
	PutDuration, _ = Meter.Float64Histogram("blocks.put.duration",
		metric.WithDescription("PUT operation latency in milliseconds"),
		metric.WithUnit("ms"))
	GetDuration, _ = Meter.Float64Histogram("blocks.get.duration",
		metric.WithDescription("GET operation latency in milliseconds"),
		metric.WithUnit("ms"))
	DeleteDuration, _ = Meter.Float64Histogram("blocks.delete.duration",
		metric.WithDescription("DELETE operation latency in milliseconds"),
		metric.WithUnit("ms"))

	// Initialize gauges
	BlocksStored, _ = Meter.Int64UpDownCounter("blocks.stored.count",
		metric.WithDescription("Current number of blocks stored"))

	return shutdown, err
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context) (*trace.TracerProvider, error) {
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
	)
	return tracerProvider, nil
}

func newMeterProvider(ctx context.Context) (*sdkmetric.MeterProvider, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)
	return meterProvider, nil
}

// TODO: Migrate this to open telemetry
func newLoggerProvider(ctx context.Context) (*log.LoggerProvider, error) {
	logExporter, err := stdoutlog.New(stdoutlog.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	return loggerProvider, nil
}
