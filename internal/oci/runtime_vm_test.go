package oci_test

import (
	"context"
	"syscall"
	"testing"

	task "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/ttrpc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/cri-o/cri-o/internal/oci"
)

func TestParseShimAddress(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "plain address string",
			input: "unix:///run/containerd/s/abc123\n",
			want:  "unix:///run/containerd/s/abc123",
		},
		{
			name:  "plain address no newline",
			input: "unix:///run/containerd/s/abc123",
			want:  "unix:///run/containerd/s/abc123",
		},
		{
			name:  "JSON BootstrapParams from gVisor shim",
			input: `{"version":2,"address":"unix:///run/containerd/s/abc123","protocol":"ttrpc"}`,
			want:  "unix:///run/containerd/s/abc123",
		},
		{
			name:  "JSON BootstrapParams with whitespace",
			input: "  {\"version\":2,\"address\":\"unix:///run/containerd/s/abc123\",\"protocol\":\"ttrpc\"}\n",
			want:  "unix:///run/containerd/s/abc123",
		},
		{
			// Like containerd's parseStartResponse, output that does not
			// unmarshal into a versioned BootstrapParams is treated as a raw
			// (legacy) address rather than an error.
			name:  "malformed JSON falls back to raw address",
			input: "{not valid json}",
			want:  "{not valid json}",
		},
		{
			// version < 2 is treated as a legacy shim: the raw output is the
			// address.
			name:  "JSON with version below 2 treated as legacy",
			input: `{"version":1,"address":"unix:///run/containerd/s/abc123"}`,
			want:  `{"version":1,"address":"unix:///run/containerd/s/abc123"}`,
		},
		{
			name:    "unsupported shim version",
			input:   `{"version":3,"address":"unix:///run/containerd/s/abc123","protocol":"ttrpc"}`,
			wantErr: true,
		},
		{
			name:  "empty output",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only",
			input: "  \n  ",
			want:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := oci.ParseShimAddress([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseShimAddress(%q) = %q, want error", tc.input, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseShimAddress(%q) error: %v", tc.input, err)
			}

			if got != tc.want {
				t.Errorf("ParseShimAddress(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// fakeTaskService is a minimal task.TaskService stub for testing kill() behavior.
// Only Kill() is implemented; all other methods panic if called.
type fakeTaskService struct {
	task.TaskService

	killErr error
}

func (f *fakeTaskService) Kill(_ context.Context, _ *task.KillRequest) (*emptypb.Empty, error) {
	return nil, f.killErr
}

var _ = t.Describe("RuntimeVM", func() {
	Describe("kill", func() {
		It("should return nil when shim has already exited (ttrpc.ErrClosed)", func() {
			// Given — shim ttrpc connection is closed (shim exited before kill was called)
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})

			// When
			err := r.Kill("ctr-id", "", syscall.SIGTERM)

			// Then — a kill on a dead shim means the container is already stopped
			Expect(err).NotTo(HaveOccurred())
		})

		It("should propagate errors that are not ttrpc.ErrClosed", func() {
			// Given — shim returns a real error (e.g. container not found)
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: ttrpc.ErrClosed})
			rErr := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: context.DeadlineExceeded})

			// Sanity: ErrClosed is swallowed
			Expect(r.Kill("ctr-id", "", syscall.SIGTERM)).NotTo(HaveOccurred())

			// Other errors are propagated
			Expect(rErr.Kill("ctr-id", "", syscall.SIGTERM)).To(HaveOccurred())
		})

		It("should return nil when kill succeeds", func() {
			// Given — shim is alive and kill succeeds
			r := oci.NewRuntimeVMWithTask(&fakeTaskService{killErr: nil})

			// When
			err := r.Kill("ctr-id", "", syscall.SIGTERM)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
