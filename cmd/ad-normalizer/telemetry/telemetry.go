package telemetry

import (
	"context"
	"errors"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func newResource(config config.AdNormalizerConfig) (*resource.Resource, error) {
	return resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			resource.Default().SchemaURL(),
			semconv.ServiceName("eyevinn/ad-normalizer"),
			semconv.ServiceVersion(config.Version),
			semconv.ServiceInstanceID(config.InstanceID),
		))
}

func SetupOtelSdk(
	ctx context.Context,
	config config.AdNormalizerConfig,
) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	shutdown = func(ctx context.Context) error {
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

	var traceExporter trace.SpanExporter
	var metricExporter metric.Exporter
	var meErr, teErr error
	if config.InstanceID == "local" {
		traceExporter, teErr = stdouttrace.New()
		metricExporter, meErr = stdoutmetric.New()
	} else {
		metricExporter, meErr = otlpmetrichttp.New(ctx)
		traceExporter, teErr = otlptracehttp.New(ctx)
	}
	if teErr != nil {
		handleErr(teErr)
		return
	}
	if meErr != nil {
		handleErr(meErr)
		return
	}

	resource, err := newResource(config)
	if err != nil {
		handleErr(err)
		return
	}

	// Set up trace provider.
	tracerProvider := newTraceProvider(resource, traceExporter)
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up metric provider
	meterProvider := newMeterProvider(resource, metricExporter)
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
func newTraceProvider(res *resource.Resource, te trace.SpanExporter) *trace.TracerProvider {
	return trace.NewTracerProvider(
		trace.WithBatcher(te),
		trace.WithResource(res),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(0.1))),
	)
}
func newMeterProvider(res *resource.Resource, me metric.Exporter) *metric.MeterProvider {
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(me, metric.WithInterval(10*time.Second))),
	)
	return meterProvider
}
