package criocli_test

import (
	"flag"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/criocli"
)

// The actual test suite.
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
			Expect(slice.Set(v)).To(Succeed())
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
		Expect(slice.Set("value1")).To(Succeed())
		Expect(slice.Set("value2")).To(Succeed())

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
			"should escape $'s within double quotes",
			"barfoo", "barfoo's usage $needs $escaped $dollars",
			"\"barfoo:barfoo's usage \\$needs \\$escaped \\$dollars\"",
		),
	)
})

// CLI Flags/Parameter test suite.
var _ = t.Describe("CLI Flags", func() {
	const flagName = "crio"

	var (
		flagSet      *flag.FlagSet
		ctx          *cli.Context
		app          *cli.App
		err          error
		commandFlags []cli.Flag
	)

	BeforeEach(func() {
		flagSet = flag.NewFlagSet(flagName, flag.ExitOnError)
		app = cli.NewApp()
		ctx = cli.NewContext(app, flagSet, nil)
	})

	It("Flag test hostnetwork-disable-selinux", func() {
		// Default Config
		app.Flags, app.Metadata, err = criocli.GetFlagsAndMetadata()
		Expect(err).ToNot(HaveOccurred())
		config, err := criocli.GetConfigFromContext(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Then
		Expect(config.RuntimeConfig.HostNetworkDisableSELinux).To(BeTrue())

		// Set Config & Merge
		setFlag := &cli.BoolFlag{
			Name:       "hostnetwork-disable-selinux",
			Value:      false,
			HasBeenSet: true,
		}
		err = setFlag.Apply(flagSet)
		Expect(err).ToNot(HaveOccurred())
		ctx.Command.Flags = append(commandFlags, setFlag)
		config, err = criocli.GetAndMergeConfigFromContext(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Then
		Expect(config.RuntimeConfig.HostNetworkDisableSELinux).To(Equal(setFlag.Value))
	})

	It("Flag test disable-hostport-mapping", func() {
		// Default Config
		app.Flags, app.Metadata, err = criocli.GetFlagsAndMetadata()
		Expect(err).ToNot(HaveOccurred())
		config, err := criocli.GetConfigFromContext(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Then
		Expect(config.RuntimeConfig.DisableHostPortMapping).To(BeFalse())

		// Set Config & Merge
		setFlag := &cli.BoolFlag{
			Name:       "disable-hostport-mapping",
			Value:      true,
			HasBeenSet: true,
		}
		err = setFlag.Apply(flagSet)
		Expect(err).ToNot(HaveOccurred())
		ctx.Command.Flags = append(commandFlags, setFlag)
		config, err = criocli.GetAndMergeConfigFromContext(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Then
		Expect(config.RuntimeConfig.DisableHostPortMapping).To(BeTrue())
	})
})
