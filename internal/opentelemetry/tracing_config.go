package opentelemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
)

const tracingServiceName = "crio"

var tracer = otel.Tracer(tracingServiceName)

func Tracer() trace.Tracer {
	return tracer
}

// InitTracing configures opentelemetry exporter and tracer provider.
func InitTracing(ctx context.Context, collectorAddress string, samplingRate int) (*sdktrace.TracerProvider, []otelgrpc.Option, error) {
	var tp *sdktrace.TracerProvider

	hostname, err := os.Hostname()
	if err != nil {
		return nil, nil, fmt.Errorf("get hostname: %w", err)
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(tracingServiceName),
		semconv.HostNameKey.String(hostname),
		semconv.ProcessPIDKey.Int64(int64(os.Getpid())),
	)

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorAddress),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, err
	}

	// Only emit spans when the kubelet sends a request with a sampled trace
	sampler := sdktrace.NeverSample()
	// Or, emit spans for a fraction of transactions
	if samplingRate > 0 {
		sampler = sdktrace.TraceIDRatioBased(float64(samplingRate) / float64(1000000))
	}
	// batch span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tp = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(res),
	)
	tmp := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(tmp)
	opts := []otelgrpc.Option{otelgrpc.WithPropagators(tmp), otelgrpc.WithTracerProvider(tp)}

	return tp, opts, nil
}
