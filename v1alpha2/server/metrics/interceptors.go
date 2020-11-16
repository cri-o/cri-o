package metrics

import (
	"context"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
)

// UnaryInterceptor adds all necessary metrics to incoming gRPC requests
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

		// the RPC
		resp, err := handler(ctx, req)

		// record the operation
		CRIOOperations.WithLabelValues(operation).Inc()
		CRIOOperationsLatency.WithLabelValues(operation).
			Observe(SinceInMicroseconds(operationStart))

		// record error metric if occurred
		if err != nil {
			CRIOOperationsErrors.WithLabelValues(operation).Inc()
		}

		return resp, err
	}
}
