package references_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.podman.io/image/v5/docker/reference"

	"github.com/cri-o/cri-o/internal/storage/references"
)

var _ = t.Describe("RegistryImageReference", func() {
	It("Should parse valid references", func() {
		ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData("minimal")
		Expect(err).ToNot(HaveOccurred())
		Expect(ref.StringForOutOfProcessConsumptionOnly()).To(Equal("docker.io/library/minimal:latest"))

		ref, err = references.ParseRegistryImageReferenceFromOutOfProcessData("quay.io/ns/repo:notlatest")
		Expect(err).ToNot(HaveOccurred())
		Expect(ref.StringForOutOfProcessConsumptionOnly()).To(Equal("quay.io/ns/repo:notlatest"))
	})

	It("Should reject invalid references", func() {
		for _, input := range []string{
			"",
			"@",
			"example.com/",
		} {
			_, err := references.ParseRegistryImageReferenceFromOutOfProcessData(input)
			Expect(err).To(HaveOccurred())
		}
	})

	It("Should reject construction of invalid values", func() {
		Expect(func() { references.RegistryImageReferenceFromRaw(nil) }).To(Panic())

		nameOnly, err := reference.ParseNormalizedNamed("example.com/ns/repo-only")
		Expect(err).ToNot(HaveOccurred())
		Expect(func() { references.RegistryImageReferenceFromRaw(nameOnly) }).To(Panic())
	})

	It("Should reject use of uninitialized/empty values", func() {
		ref := references.RegistryImageReference{}
		Expect(func() { _ = ref.StringForOutOfProcessConsumptionOnly() }).To(Panic())
	})

	It("Should be usable for logging, but not otherwise expose a string value", func() {
		const testName = "quay.io/ns/repo:notlatest"

		ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testName)
		Expect(err).ToNot(HaveOccurred())

		var _ fmt.Formatter = ref // A compile-time check that ref implements Formatter

		// We need an intermediate any() value, otherwise Go refuses to allow a compile-time-known check to be done at runtime.
		_, isStringer := any(ref).(fmt.Stringer)
		Expect(isStringer).To(BeFalse())

		res := fmt.Sprintf("%s", ref)
		Expect(res).To(Equal(testName))
		res = fmt.Sprintf("%q", ref)
		Expect(res).To(Equal(`"` + testName + `"`))
	})

	It("Should return a plausible raw value", func() {
		const testName = "quay.io/ns/repo:notlatest"

		ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(testName)
		Expect(err).ToNot(HaveOccurred())

		raw := ref.Raw()
		Expect(raw.String()).To(Equal(testName))
	})

	It("Should return a correct registry value", func() {
		for _, c := range []struct{ in, expected string }{
			{"implied", "docker.io"},
			{"example.com/foo:tag", "example.com"},
			{"example.com:8000/foo:tag", "example.com:8000"},
		} {
			ref, err := references.ParseRegistryImageReferenceFromOutOfProcessData(c.in)
			Expect(err).ToNot(HaveOccurred())
			registry := ref.Registry()
			Expect(registry).To(Equal(c.expected))
		}
	})
})
