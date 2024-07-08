package cgmgr_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
)

const (
	sbID                 = "sbid"
	cID                  = "cid"
	genericSandboxParent = "sb-parent"
	systemdManager       = "systemd"
	cgroupfsManager      = "cgroupfs"
)

// The actual test suite.
var _ = t.Describe("Cgmgr", func() {
	var sut cgmgr.CgroupManager

	BeforeEach(func() {
		sut = cgmgr.New()
		Expect(sut).NotTo(BeNil())
	})

	t.Describe("SetCgroupManager", func() {
		It("should be non nil by default", func() {
			// Given
			// When
			// Then
			Expect(sut).To(Not(BeNil()))
		})
		It("should be able to be set to cgroupfs", func() {
			// Given
			// When
			var err error
			sut, err = cgmgr.SetCgroupManager(cgroupfsManager)

			// Then
			Expect(sut).To(Not(BeNil()))
			Expect(err).ToNot(HaveOccurred())
		})
		It("should be able to be set to systemd", func() {
			// Given
			// When
			var err error
			sut, err = cgmgr.SetCgroupManager(systemdManager)

			// Then
			Expect(sut).To(Not(BeNil()))
			Expect(err).ToNot(HaveOccurred())
		})
		It("should fail when invalid", func() {
			// Given
			// When
			var err error
			sut, err = cgmgr.SetCgroupManager("invalid")

			// Then
			Expect(sut).To(BeNil())
			Expect(err).To(HaveOccurred())
		})
	})
	t.Describe("Name", func() {
		It("should be systemd per default", func() {
			// Given
			// When
			name := sut.Name()

			// Then
			Expect(name).To(Equal(systemdManager))
		})
		It("should be able to be set to cgroupfs", func() {
			// Given
			var err error
			sut, err = cgmgr.SetCgroupManager(cgroupfsManager)
			Expect(sut).To(Not(BeNil()))
			Expect(err).ToNot(HaveOccurred())
			// When
			name := sut.Name()

			// Then
			Expect(name).To(Equal(cgroupfsManager))
		})
	})
	t.Describe("IsSystemd", func() {
		It("should be systemd per default", func() {
			// Given
			// When
			res := sut.IsSystemd()

			// Then
			Expect(res).To(BeTrue())
		})
		It("should be able to be set to cgroupfs", func() {
			// Given
			var err error
			sut, err = cgmgr.SetCgroupManager(cgroupfsManager)
			Expect(sut).To(Not(BeNil()))
			Expect(err).ToNot(HaveOccurred())
			// When
			res := sut.IsSystemd()

			// Then
			Expect(res).To(BeFalse())
		})
	})
	t.Describe("CgroupfsManager", func() {
		BeforeEach(func() {
			sut = new(cgmgr.CgroupfsManager)
		})
		t.Describe("ContainerCgroupPath", func() {
			It("should contain default /crio", func() {
				// Given
				// When
				cgroupPath := sut.ContainerCgroupPath("", cID)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cID))
				Expect(cgroupPath).To(ContainSubstring("/crio"))
			})
			It("can override sandbox parent", func() {
				// Given
				// When
				cgroupPath := sut.ContainerCgroupPath(genericSandboxParent, cID)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cID))
				Expect(cgroupPath).To(ContainSubstring(genericSandboxParent))
			})
		})
		t.Describe("SandboxCgroupPath", func() {
			It("should fail if sandbox parent has .slice", func() {
				// Given
				sbParent := "sandbox-parent.slice"
				// When
				cgParent, cgPath, err := sut.SandboxCgroupPath(sbParent, sbID, int64(0))

				// Then
				Expect(cgParent).To(BeEmpty())
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(HaveOccurred())
			})
			It("can override sandbox parent", func() {
				// Given
				// When
				cgParent, cgPath, err := sut.SandboxCgroupPath(genericSandboxParent, sbID, int64(0))

				// Then
				Expect(cgParent).To(Equal(genericSandboxParent))
				Expect(cgPath).To(ContainSubstring(genericSandboxParent))
				Expect(cgPath).To(ContainSubstring(sbID))
				Expect(err).ToNot(HaveOccurred())
			})
		})
		t.Describe("MoveConmonToCgroup", func() {
			It("should fail if invalid conmon cgroup", func() {
				// Given
				conmonCgroup := "notPodOrEmpty"
				// When
				cgPath, err := sut.MoveConmonToCgroup("", "", conmonCgroup, 0, nil)

				// Then
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(HaveOccurred())
			})
		})
	})
	t.Describe("SystemdManager", func() {
		t.Describe("ContainerCgroupPath", func() {
			It("should contain default system.slice", func() {
				// Given
				// When
				cgroupPath := sut.ContainerCgroupPath("", cID)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cID))
				Expect(cgroupPath).To(ContainSubstring("system.slice"))
			})
			It("can override sandbox parent", func() {
				// Given
				// When
				cgroupPath := sut.ContainerCgroupPath(genericSandboxParent, cID)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cID))
				Expect(cgroupPath).To(ContainSubstring(genericSandboxParent))
			})
		})
		t.Describe("ContainerCgroupAbsolutePath", func() {
			It("should contain default system.slice", func() {
				// Given
				// When
				cgroupPath, err := sut.ContainerCgroupAbsolutePath("", cID)
				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(cgroupPath).To(ContainSubstring(cID))
				Expect(cgroupPath).To(ContainSubstring("system.slice"))
			})
			It("should be an absolute path", func() {
				// Given
				// When
				cgroupPath, err := sut.ContainerCgroupAbsolutePath("", cID)
				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(filepath.IsAbs(cgroupPath)).To(BeTrue())
			})
			It("should fail to expand slice", func() {
				// Given
				// When
				cgroupPath, err := sut.ContainerCgroupAbsolutePath("::::", cID)
				// Then
				Expect(err).To(HaveOccurred())
				Expect(cgroupPath).To(Equal(""))
			})
		})
		t.Describe("SandboxCgroupPath", func() {
			It("should fail when parent too short", func() {
				// Given
				sbParent := "slice"
				// When
				cgParent, cgPath, err := sut.SandboxCgroupPath(sbParent, sbID, int64(0))

				// Then
				Expect(cgParent).To(BeEmpty())
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(HaveOccurred())
			})
			It("should fail when parent not slice", func() {
				// Given
				sbParent := "systemd.invalid"
				// When
				cgParent, cgPath, err := sut.SandboxCgroupPath(sbParent, sbID, int64(0))

				// Then
				Expect(cgParent).To(BeEmpty())
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(HaveOccurred())
			})
			It("should fail container minimum memory limit check", func() {
				// When
				err := cgmgr.VerifyMemoryIsEnough(100, 200)

				// Then
				Expect(err).To(HaveOccurred())
			})
			It("should not fail container minimum memory limit check", func() {
				// When
				err := cgmgr.VerifyMemoryIsEnough(151, 150)

				// Then
				Expect(err).ToNot(HaveOccurred())
			})
		})
		t.Describe("MoveConmonToCgroup", func() {
			It("should fail if invalid conmon cgroup", func() {
				// Given
				conmonCgroup := "notPodOrEmpty"
				// When
				cgPath, err := sut.MoveConmonToCgroup("", "", conmonCgroup, -1, nil)

				// Then
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
