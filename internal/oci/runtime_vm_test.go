package oci_test

import (
	"testing"

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
			name:    "invalid JSON starting with brace",
			input:   "{not valid json}",
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
