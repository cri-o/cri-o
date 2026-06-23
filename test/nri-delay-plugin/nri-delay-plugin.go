package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/containerd/nri/pkg/api"
	"github.com/containerd/nri/pkg/stub"
)

const (
	pluginName = "delay-plugin"
	// delayAnnotation is the pod annotation key used to configure the delay duration.
	// Format: time.Duration string (e.g., "10s", "5s", "500ms")
	// Default: 10s if annotation is not present or invalid.
	delayAnnotation = "nri-delay-plugin/delay"
	// logFileAnnotation is the pod annotation key for specifying log file path.
	logFileAnnotation = "nri-delay-plugin/log-file"
	defaultDelay      = 10 * time.Second
)

type plugin struct {
	stub stub.Stub
}

//nolint:unparam // Interface method - error return required by NRI plugin interface
func (p *plugin) Configure(_ context.Context, config, runtime, version string) (stub.EventMask, error) {
	log.Printf("Configure: config=%s, runtime=%s, version=%s", config, runtime, version)

	return api.MustParseEventMask("RunPodSandbox"), nil
}

//nolint:unparam // Interface method - return types required by NRI plugin interface
func (p *plugin) Synchronize(_ context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	log.Printf("Synchronize: %d pods, %d containers", len(pods), len(containers))

	return nil, nil
}

func (p *plugin) RunPodSandbox(ctx context.Context, pod *api.PodSandbox) error {
	// Check for custom delay and log file in pod annotations
	delay := defaultDelay

	if pod.GetAnnotations() != nil {
		// Check for custom delay
		if delayStr, ok := pod.GetAnnotations()[delayAnnotation]; ok {
			if d, err := time.ParseDuration(delayStr); err == nil && d > 0 {
				delay = d
				log.Printf("RunPodSandbox: using custom delay from annotation: %s", delay)
			} else {
				log.Printf("RunPodSandbox: invalid delay annotation '%s', using default: %s", delayStr, delay)
			}
		}

		// Check for log file path
		if logPath, ok := pod.GetAnnotations()[logFileAnnotation]; ok && logPath != "" {
			if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				defer logFile.Close()

				// Write pod ID and timing info to log file
				fmt.Fprintf(logFile, "pod_id=%s\n", pod.GetId())

				if err := logFile.Sync(); err != nil {
					log.Printf("Warning: failed to sync log file: %v", err)
				}

				fmt.Fprintf(logFile, "delay_start=%d\n", time.Now().Unix())

				defer func() {
					fmt.Fprintf(logFile, "delay_end=%d\n", time.Now().Unix())
					fmt.Fprintf(logFile, "delay_duration=%d\n", int(delay.Seconds()))

					if err := logFile.Sync(); err != nil {
						log.Printf("Warning: failed to sync log file: %v", err)
					}
				}()
			}
		}
	}

	log.Printf("RunPodSandbox: pod=%s/%s - DELAYING %s", pod.GetNamespace(), pod.GetName(), delay)

	// Context-aware sleep to respect cancellation
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		log.Printf("RunPodSandbox: pod=%s/%s - DELAY COMPLETE", pod.GetNamespace(), pod.GetName())

		return nil
	case <-ctx.Done():
		log.Printf("RunPodSandbox: pod=%s/%s - DELAY CANCELLED", pod.GetNamespace(), pod.GetName())

		return ctx.Err()
	}
}

func (p *plugin) Shutdown(ctx context.Context) {
	log.Println("Shutdown")
}

func main() {
	var (
		err error
		p   plugin
	)

	// Log to stderr so CRI-O captures it in its logs
	log.SetOutput(os.Stderr)
	log.SetPrefix("[nri-delay-plugin] ")

	log.Printf("Starting NRI delay plugin %s", pluginName)

	p.stub, err = stub.New(&p,
		stub.WithPluginName(pluginName),
	)
	if err != nil {
		log.Fatalf("failed to create plugin stub: %v", err)
	}

	log.Printf("Plugin stub created successfully")

	err = p.stub.Run(context.Background())
	if err != nil {
		log.Fatalf("plugin exited with error: %v", err)
	}

	log.Printf("Plugin exiting normally")
}
