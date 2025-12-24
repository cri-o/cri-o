package server

import (
	"fmt"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// BenchmarkMergeEnvs benchmarks the optimized mergeEnvs function
// to demonstrate performance improvement over the original O(N*M) implementation.
func BenchmarkMergeEnvs(b *testing.B) {
	// Simulate realistic container environment:
	// - 10 image env vars (common for base images)
	// - 20 kube env vars (typical for application containers)
	configImage := &v1.Image{
		Config: v1.ImageConfig{
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"HOSTNAME=container",
				"HOME=/root",
				"LANG=C.UTF-8",
				"GPG_KEY=ABC123",
				"PYTHON_VERSION=3.9.0",
				"PYTHON_PIP_VERSION=20.2.3",
				"PYTHON_GET_PIP_URL=https://github.com/pypa/get-pip",
				"PYTHON_GET_PIP_SHA256=abc123",
				"APP_VERSION=1.0.0",
			},
		},
	}

	configKube := []*types.KeyValue{
		{Key: "APP_ENV", Value: "production"},
		{Key: "APP_PORT", Value: "8080"},
		{Key: "DB_HOST", Value: "postgres"},
		{Key: "DB_PORT", Value: "5432"},
		{Key: "REDIS_HOST", Value: "redis"},
		{Key: "REDIS_PORT", Value: "6379"},
		{Key: "LOG_LEVEL", Value: "info"},
		{Key: "METRICS_ENABLED", Value: "true"},
		{Key: "TRACING_ENABLED", Value: "true"},
		{Key: "SERVICE_NAME", Value: "api-server"},
		{Key: "POD_NAME", Value: "api-server-abc123"},
		{Key: "POD_NAMESPACE", Value: "default"},
		{Key: "POD_IP", Value: "10.244.0.1"},
		{Key: "NODE_NAME", Value: "worker-1"},
		{Key: "AWS_REGION", Value: "us-east-1"},
		{Key: "S3_BUCKET", Value: "my-bucket"},
		{Key: "QUEUE_URL", Value: "https://sqs.amazonaws.com"},
		{Key: "API_KEY", Value: "secret"},
		{Key: "FEATURE_FLAG_A", Value: "enabled"},
		{Key: "PATH", Value: "/app/bin:/usr/local/bin:/usr/bin:/bin"}, // Override
	}

	b.ResetTimer()

	for b.Loop() {
		_ = mergeEnvs(configImage, configKube)
	}
}

// BenchmarkMergeEnvsWorstCase benchmarks the worst case where no env vars overlap.
func BenchmarkMergeEnvsWorstCase(b *testing.B) {
	configImage := &v1.Image{
		Config: v1.ImageConfig{
			Env: make([]string, 50),
		},
	}
	for i := range 50 {
		configImage.Config.Env[i] = fmt.Sprintf("IMAGE_VAR_%d=value", i)
	}

	configKube := make([]*types.KeyValue, 50)
	for i := range 50 {
		configKube[i] = &types.KeyValue{
			Key:   fmt.Sprintf("KUBE_VAR_%d", i),
			Value: "value",
		}
	}

	b.ResetTimer()

	for b.Loop() {
		_ = mergeEnvs(configImage, configKube)
	}
}
