module go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc

go 1.15

replace go.opentelemetry.io/contrib => ../../../../

require (
	github.com/golang/protobuf v1.5.2
	go.opentelemetry.io/otel v1.1.0
	go.opentelemetry.io/otel/trace v1.1.0
	google.golang.org/grpc v1.41.0
)
