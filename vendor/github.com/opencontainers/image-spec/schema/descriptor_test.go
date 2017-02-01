// Copyright 2016 The Linux Foundation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema_test

import (
	"strings"
	"testing"

	"github.com/opencontainers/image-spec/schema"
)

func TestDescriptor(t *testing.T) {
	for i, tt := range []struct {
		descriptor string
		fail     bool
	}{
		// valid descriptor
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: false,
		},

		// expected failure: mediaType missing
		{
			descriptor: `
{
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: mediaType does not match pattern (no subtype)
		{
			descriptor: `
{
  "mediaType": "application",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: mediaType does not match pattern (invalid first type character)
		{
			descriptor: `
{
  "mediaType": ".foo/bar",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: mediaType does not match pattern (invalid first subtype character)
		{
			descriptor: `
{
  "mediaType": "foo/.bar",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected success: mediaType has type and subtype as long as possible
		{
			descriptor: `
{
  "mediaType": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567/1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: false,
		},

		// expected success: mediaType does not match pattern (type too long)
		{
			descriptor: `
{
  "mediaType": "12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678/bar",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected success: mediaType does not match pattern (subtype too long)
		{
			descriptor: `
{
  "mediaType": "foo/12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678",
  "size": 7682,
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: size missing
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: size is a string, expected integer
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": "7682",
  "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: digest missing
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682
}
`,
			fail: true,
		},

		// expected failure: digest does not match pattern (no algorithm)
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682,
  "digest": ":5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: digest does not match pattern (no hash)
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682,
  "digest": "sha256"
}
`,
			fail: true,
		},

		// expected failure: digest does not match pattern (invalid aglorithm characters)
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682,
  "digest": "SHA256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270"
}
`,
			fail: true,
		},

		// expected failure: digest does not match pattern (invalid hash characters)
		{
			descriptor: `
{
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "size": 7682,
  "digest": "sha256:5B0BCABD1ED22E9FB1310CF6C2DEC7CDEF19F0AD69EFA1F392E94A4333501270"
}
`,
			fail: true,
		},
	} {
		r := strings.NewReader(tt.descriptor)
		err := schema.MediaTypeDescriptor.Validate(r)

		if got := err != nil; tt.fail != got {
			t.Errorf("test %d: expected validation failure %t but got %t, err %v", i, tt.fail, got, err)
		}
	}
}
