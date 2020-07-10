package conmonmgr

import (
	runnerMock "github.com/cri-o/cri-o/test/mocks/cmdrunner"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

const validPath = "/bin/ls"

// The actual test suite
var _ = t.Describe("ConmonManager", func() {
	var runner *runnerMock.MockCommandRunner
	t.Describe("New", func() {
		BeforeEach(func() {
			runner = runnerMock.NewMockCommandRunner(mockCtrl)
		})
		It("should fail when path not absolute", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().CombinedOutput(gomock.Any(), gomock.Any()).Return([]byte{}, nil),
			)

			// When
			mgr, err := newWithCommandRunner("", runner)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(mgr).To(BeNil())
		})
		It("should fail when command fails", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().CombinedOutput(gomock.Any(), gomock.Any()).Return([]byte{}, errors.New("cmd failed")),
			)

			// When
			mgr, err := newWithCommandRunner(validPath, runner)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(mgr).To(BeNil())
		})
		It("should fail when output unexpected", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().CombinedOutput(gomock.Any(), gomock.Any()).Return([]byte("unexpected"), nil),
			)

			// When
			mgr, err := newWithCommandRunner(validPath, runner)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(mgr).To(BeNil())
		})
		It("should succeed when output expected", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().CombinedOutput(gomock.Any(), gomock.Any()).Return([]byte("conmon version 2.0.0"), nil),
			)

			// When
			mgr, err := newWithCommandRunner(validPath, runner)

			// Then
			Expect(err).To(BeNil())
			Expect(mgr).ToNot(BeNil())
		})
		It("should succeed when output expected", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().CombinedOutput(gomock.Any(), gomock.Any()).Return([]byte("conmon version 2.0.0"), nil),
			)

			// When
			mgr, err := newWithCommandRunner(validPath, runner)

			// Then
			Expect(err).To(BeNil())
			Expect(mgr).ToNot(BeNil())
		})
	})
	t.Describe("parseConmonVersion", func() {
		var mgr *ConmonManager
		const invalidNumber = "invalid"
		const validNumber = "0"
		BeforeEach(func() {
			mgr = new(ConmonManager)
		})
		It("should fail when major not number", func() {
			// When
			err := mgr.parseConmonVersion(invalidNumber, validNumber, validNumber)
			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when minor not number", func() {
			// When
			err := mgr.parseConmonVersion(validNumber, invalidNumber, validNumber)
			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail when patch not number", func() {
			// When
			err := mgr.parseConmonVersion(validNumber, validNumber, invalidNumber)
			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should succeed when all are numbers", func() {
			// When
			err := mgr.parseConmonVersion(validNumber, validNumber, validNumber)
			// Then
			Expect(err).To(BeNil())
		})
	})
	t.Describe("initializeSupportsSync", func() {
		var mgr *ConmonManager
		BeforeEach(func() {
			mgr = new(ConmonManager)
		})
		It("should be false when major version less", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync - 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(false))
		})
		It("should be true when major version greater", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync + 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(true))
		})
		It("should be false when minor version less", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync
			mgr.minorVersion = minorVersionSupportsSync - 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(false))
		})
		It("should be true when minor version greater", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync
			mgr.minorVersion = minorVersionSupportsSync + 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(true))
		})
		It("should be false when patch version less", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync
			mgr.minorVersion = minorVersionSupportsSync
			mgr.patchVersion = patchVersionSupportsSync - 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(false))
		})
		It("should be true when patch version greater", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync
			mgr.minorVersion = minorVersionSupportsSync
			mgr.patchVersion = patchVersionSupportsSync + 1

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(true))
		})
		It("should be true when version equal", func() {
			// Given
			mgr.majorVersion = majorVersionSupportsSync
			mgr.minorVersion = minorVersionSupportsSync
			mgr.patchVersion = patchVersionSupportsSync

			// When
			mgr.initializeSupportsSync()

			// Then
			Expect(mgr.SupportsSync()).To(Equal(true))
		})
	})
})
