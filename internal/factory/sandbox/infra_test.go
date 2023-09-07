package sandbox_test

import (
	"os"
	"path/filepath"

	sboxfactory "github.com/cri-o/cri-o/internal/factory/sandbox"
	libsandbox "github.com/cri-o/cri-o/internal/lib/sandbox"

	"github.com/cri-o/cri-o/pkg/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	defaultDNSPath  = "/etc/resolv.conf"
	testDNSPath     = "fixtures/resolv_test.conf"
	dnsPath         = "fixtures/resolv.conf"
	expandedDNSPath = "fixtures/expanded_resolv.conf"
)

var _ = Describe("Sandbox", func() {
	Context("ParseDNSOptions", func() {
		testCases := []struct {
			Servers, Searches, Options []string
			Path                       string
			Want                       string
		}{
			{
				[]string{},
				[]string{},
				[]string{},
				testDNSPath, defaultDNSPath,
			},
			{
				[]string{"cri-o.io", "github.com"},
				[]string{"192.30.253.113", "192.30.252.153"},
				[]string{"timeout:5", "attempts:3"},
				testDNSPath, dnsPath,
			},
			{
				[]string{"cri-o.io", "github.com"},
				[]string{"1.com", "2.com", "3.com", "4.com", "5.com", "6.com", "7.com"},
				[]string{"timeout:5", "attempts:3"},
				testDNSPath, expandedDNSPath,
			},
		}

		for _, c := range testCases {
			err := sboxfactory.ParseDNSOptions(c.Servers, c.Searches, c.Options, c.Path)
			defer os.Remove(c.Path)
			Expect(err).To(BeNil())

			expect, _ := os.ReadFile(c.Want) // nolint: errcheck
			result, _ := os.ReadFile(c.Path) // nolint: errcheck
			Expect(result).To(Equal(expect))
		}
	})

	Context("PauseCommand", func() {
		var cfg *config.Config

		BeforeEach(func() {
			// Given
			var err error
			cfg, err = config.DefaultConfig()
			Expect(err).To(BeNil())
		})

		It("should succeed with default config", func() {
			// When
			_, err := sboxfactory.PauseCommand(cfg, nil)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with Entrypoint", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := []string{"/custom-pause"}
			image := &v1.Image{Config: v1.ImageConfig{Entrypoint: entrypoint}}

			// When
			res, err := sboxfactory.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(entrypoint))
		})

		It("should succeed with Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			cmd := []string{"some-cmd"}
			image := &v1.Image{Config: v1.ImageConfig{Cmd: cmd}}

			// When
			res, err := sboxfactory.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(Equal(cmd))
		})

		It("should succeed with Entrypoint and Cmd", func() {
			// Given
			cfg.PauseCommand = ""
			entrypoint := "/custom-pause"
			cmd := "some-cmd"
			image := &v1.Image{Config: v1.ImageConfig{
				Entrypoint: []string{entrypoint},
				Cmd:        []string{cmd},
			}}

			// When
			res, err := sboxfactory.PauseCommand(cfg, image)

			// Then
			Expect(err).To(BeNil())
			Expect(res).To(HaveLen(2))
			Expect(res[0]).To(Equal(entrypoint))
			Expect(res[1]).To(Equal(cmd))
		})

		It("should fail if config is nil", func() {
			// When
			res, err := sboxfactory.PauseCommand(nil, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})

		It("should fail if image config is nil", func() {
			// Given
			cfg.PauseCommand = ""

			// When
			res, err := sboxfactory.PauseCommand(cfg, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})

	Context("SetupShm", func() {
		shmSize := int64(libsandbox.DefaultShmSize)
		mountLabel := "system_u:object_r:container_file_t:s0:c1,c2"

		It("should fail if shm size is <= 0", func() {
			// When
			shmSize = int64(-1)
			dir, err := os.MkdirTemp("/tmp", "shmsetup-test")
			Expect(err).To(BeNil())

			// When
			res, err := sboxfactory.SetupShm(dir, mountLabel, shmSize)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail if mount label is empty", func() {
			// When
			mountLabel = ""
			dir, err := os.MkdirTemp("/tmp", "shmsetup-test")
			Expect(err).To(BeNil())

			// When
			res, err := sboxfactory.SetupShm(dir, mountLabel, shmSize)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})

		It("should fail if dir already exists", func() {
			// Given
			dir, err := os.MkdirTemp("/tmp", "shmsetup-test")
			Expect(err).To(BeNil())
			shmPath := filepath.Join(dir, "shm")
			err = os.Mkdir(shmPath, 0o700)
			Expect(err).To(BeNil())

			// When
			res, err := sboxfactory.SetupShm(dir, mountLabel, shmSize)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})
})
