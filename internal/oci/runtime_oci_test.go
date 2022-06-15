package oci_test

import (
	"context"
	"io/ioutil"

	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Oci", func() {
	Context("TruncateAndReadFile", func() {
		tests := []struct {
			title    string
			contents []byte
			expected []byte
			fail     bool
			size     int64
		}{
			{
				title:    "should read file if size is smaller than limit",
				contents: []byte("abcd"),
				expected: []byte("abcd"),
				size:     5,
			},
			{
				title:    "should read only size if size is same as limit",
				contents: []byte("abcd"),
				expected: []byte("abcd"),
				size:     4,
			},
			{
				title:    "should read only size if size is larger than limit",
				contents: []byte("abcd"),
				expected: []byte("abc"),
				size:     3,
			},
		}
		for _, test := range tests {
			test := test
			It(test.title, func() {
				fileName := t.MustTempFile("to-read")
				Expect(ioutil.WriteFile(fileName, test.contents, 0644)).To(BeNil())
				found, err := oci.TruncateAndReadFile(context.Background(), fileName, test.size)
				Expect(err).To(BeNil())
				Expect(found).To(Equal(test.expected))
			})
		}
	})
})
