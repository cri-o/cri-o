package runtimehandlerhooks

import (
	"reflect"
	"testing"

	"k8s.io/utils/cpuset"
)

func TestMapByteToCPUSet(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected cpuset.CPUSet
	}{
		{
			name:     "Single byte, all CPUs set",
			input:    []byte{0xFF}, // 11111111 -> CPUs 0-7
			expected: cpuset.New(0, 1, 2, 3, 4, 5, 6, 7),
		},
		{
			name:     "Single byte, alternate CPUs set",
			input:    []byte{0xAA}, // 10101010 -> CPUs 1, 3, 5, 7
			expected: cpuset.New(1, 3, 5, 7),
		},
		{
			name:     "Two bytes, mixed CPUs set",
			input:    []byte{0x01, 0x02}, // 00000001 00000010 -> CPUs 0, 9
			expected: cpuset.New(0, 9),
		},
		{
			name:     "Two bytes, all CPUs set",
			input:    []byte{0xFF, 0xFF}, // 11111111 11111111 -> CPUs 0-15
			expected: cpuset.New(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15),
		},
		{
			name:  "Four bytes, mixed CPUs set",
			input: []byte{0x81, 0x24, 0x18, 0xC3},
			// Byte 0:  0x81 -> 10000001 -> CPUs 0, 7
			// Byte 1:  0x24 -> 00100100 -> CPUs 10, 13
			// Byte 2:  0x18 -> 00011000 -> CPUs 19, 20
			// Byte 3:  0xC3 -> 11000011 -> CPUs 24, 25, 30, 31
			expected: cpuset.New(0, 7, 10, 13, 19, 20, 24, 25, 30, 31),
		},
		{
			name:     "Empty input",
			input:    []byte{},
			expected: cpuset.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapByteToCPUSet(tt.input)
			if !result.Equals(tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMapHexCharToByte(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
		wantErr  bool
	}{
		{
			name:     "Valid CPU mask with even-length hex, split into 8 characters",
			input:    "ffffffff,ffffcffc",                                    // Two chunks, each 8 hex characters
			expected: []byte{0xfc, 0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // Reverse order of bytes (8 bytes)
			wantErr:  false,
		},
		{
			name:     "Valid CPU mask with multiple chunks, split into 8 characters",
			input:    "1a2b3c4d,5e6f7a8b,9c0d1e2f",                                                   // Multiple chunks
			expected: []byte{0x2f, 0x1e, 0x0d, 0x9c, 0x8b, 0x7a, 0x6f, 0x5e, 0x4d, 0x3c, 0x2b, 0x1a}, // Reverse order of bytes
			wantErr:  false,
		},
		{
			name:     "Single CPU mask character, split into 8 characters",
			input:    "f,00000000", // Single character and zero padding
			expected: []byte{0x00, 0x00, 0x00, 0x00, 0x0f},
			wantErr:  false,
		},
		{
			name:     "Empty CPU mask input",
			input:    "",
			expected: []byte{},
			wantErr:  false,
		},
		{
			name:    "Invalid CPU mask with non-hex characters",
			input:   "1g2h3i,4j5k6l", // 'g', 'h', 'i', 'j', 'k', 'l' are invalid
			wantErr: true,
		},
		{
			name:    "Invalid CPU mask with wrong format",
			input:   "xyz,abc", // Non-hex string should trigger an error
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapHexCharToByte(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("mapHexCharToByte() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("mapHexCharToByte() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
