package log

import (
	"context"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/opentelemetry"
	"github.com/cri-o/cri-o/server/metrics"
)

type ServerStream struct {
	grpc.ServerStream
	NewContext context.Context
}

func (w *ServerStream) Context() context.Context {
	return w.NewContext
}

func NewServerStream(stream grpc.ServerStream) *ServerStream {
	if existing, ok := stream.(*ServerStream); ok {
		return existing
	}
	return &ServerStream{ServerStream: stream, NewContext: stream.Context()}
}

func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		newCtx := AddRequestNameAndID(stream.Context(), info.FullMethod)
		newStream := NewServerStream(stream)
		newStream.NewContext = newCtx //nolint:fatcontext // the added context is intended here

		err := handler(srv, newStream)
		if err != nil {
			log.Debugf(newCtx, "stream error: %+v", err)
		}

		return err
	}
}

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// start values
		operationStart := time.Now()
		operation := filepath.Base(info.FullMethod)
		newCtx, span := opentelemetry.Tracer().Start(AddRequestNameAndID(ctx, info.FullMethod), info.FullMethod)
		log.Debugf(newCtx, "Request: %+v", req)

		resp, err := handler(newCtx, req)
		// record the operation
		metrics.Instance().MetricOperationsInc(operation)
		metrics.Instance().MetricOperationsLatencySet(operation, operationStart)
		metrics.Instance().MetricOperationsLatencyTotalObserve(operation, operationStart)

		if err != nil {
			log.Debugf(newCtx, "Response error: %+v", err)
			metrics.Instance().MetricOperationsErrorsInc(operation)
		} else {
			log.Debugf(newCtx, "Response: %+v", resp)
		}

		span.End()
		return resp, err
	}
}

func AddRequestNameAndID(ctx context.Context, name string) context.Context {
	return addRequestName(addRequestID(ctx), name)
}

func addRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, log.ID{}, uuid.New().String())
}

func addRequestName(ctx context.Context, req string) context.Context {
	return context.WithValue(ctx, log.Name{}, req)
}
