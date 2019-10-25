package log

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
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
		newCtx := addRequestID(stream.Context())
		newStream := NewServerStream(stream)
		newStream.NewContext = newCtx

		err := handler(srv, newStream)

		if err != nil {
			Debugf(newCtx, "stream error: %+v", err)
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
		newCtx := addRequestID(ctx)
		Debugf(newCtx, "request: %+v", req)

		resp, err := handler(newCtx, req)

		if err != nil {
			Debugf(newCtx, "response error: %+v", err)
		} else {
			Debugf(newCtx, "response: %+v", resp)
		}

		return resp, err
	}
}

func addRequestID(ctx context.Context) context.Context {
	return context.WithValue(ctx, ID{}, uuid.New().String())
}
