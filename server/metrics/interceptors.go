package metrics

import (
	"context"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
)

// UnaryInterceptor adds all necessary metrics to incoming gRPC requests.
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
		Instance().MetricOperationsInc(operation)
		Instance().MetricOperationsLatencySet(operation, operationStart)
		Instance().MetricOperationsLatencyTotalObserve(operation, operationStart)

		// record error metric if occurred
		if err != nil {
			Instance().MetricOperationsErrorsInc(operation)
		}

		return resp, err
	}
}
