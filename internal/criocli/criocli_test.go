package criocli_test

import (
	"flag"

	"github.com/cri-o/cri-o/internal/criocli"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

// The actual test suite
var _ = t.Describe("CLI", func() {
	const flagName = "flag"

	var (
		slice *cli.StringSlice
		ctx   *cli.Context
	)

	BeforeEach(func() {
		flagSet := flag.NewFlagSet(flagName, flag.ExitOnError)
		slice = cli.NewStringSlice()
		flagSet.Var(slice, flagName, "")
		ctx = cli.NewContext(nil, flagSet, nil)
	})

	DescribeTable("should parse comma separated flags", func(values ...string) {
		// Given
		for _, v := range values {
			Expect(slice.Set(v)).To(BeNil())
		}

		// When
		res := criocli.StringSliceTrySplit(ctx, flagName)

		// Then
		Expect(res).NotTo(BeNil())
		Expect(res).To(HaveLen(3))
		Expect(res).To(ContainElements("a", "b", "c"))
	},
		Entry("dense", "a,b,c"),
		Entry("trim", "   a   , b   ,    c"),
		Entry("separated", "a", "b", "c"),
	)

	It("should return a copy of the slice", func() {
		// Given
		Expect(slice.Set("value1")).To(BeNil())
		Expect(slice.Set("value2")).To(BeNil())

		// When
		res := criocli.StringSliceTrySplit(ctx, flagName)
		res[0] = "value3"

		// Then
		Expect(slice.Value()[0]).To(Equal("value1"))
	})
})

// The actual test suite for zsh completion quoting.
var _ = t.Describe("completion generation", func() {
	DescribeTable("should quote and escape strings correctly", func(name, usage, expected string) {
		// When
		result := criocli.ZshQuoteCmd(name, usage)

		// Then
		Expect(result).To(Equal(expected))
	},
		Entry(
			"should use single quotes by default",
			"foo", "description of foo",
			"'foo:description of foo'",
		),
		Entry(
			"should use double quotes for strings containing single quotes",
			"bar", "bar's description",
			"\"bar:bar's description\"",
		),
		Entry(
			"should not escape $'s within single quotes",
			"foobar", "foobar handles $FOOBAR",
			"'foobar:foobar handles $FOOBAR'",
		),
		Entry(
			"should escape $'s within doube quotes",
			"barfoo", "barfoo's usage $needs $escaped $dollars",
			"\"barfoo:barfoo's usage \\$needs \\$escaped \\$dollars\"",
		),
	)
})
