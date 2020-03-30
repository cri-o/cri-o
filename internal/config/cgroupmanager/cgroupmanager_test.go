package cgroupmanager_test

import (
	"github.com/cri-o/cri-o/internal/config/cgroupmanager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	var sut cgroupmanager.CgroupManager

	BeforeEach(func() {
		sut = cgroupmanager.New()
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
			sut, err = cgroupmanager.SetCgroupManager(cgroupmanager.CgroupfsCgroupManager)

			// Then
			Expect(sut).To(Not(BeNil()))
			Expect(err).To(BeNil())
		})
		It("should be able to be set to systemd", func() {
			// Given
			// When
			var err error
			sut, err = cgroupmanager.SetCgroupManager(cgroupmanager.SystemdCgroupManager)

			// Then
			Expect(sut).To(Not(BeNil()))
			Expect(err).To(BeNil())
		})
		It("should fail when invalid", func() {
			// Given
			// When
			var err error
			sut, err = cgroupmanager.SetCgroupManager("invalid")

			// Then
			Expect(sut).To(BeNil())
			Expect(err).To(Not(BeNil()))
		})
	})
	t.Describe("Name", func() {
		It("should be systemd per default", func() {
			// Given
			// When
			name := sut.Name()

			// Then
			Expect(name).To(Equal(cgroupmanager.SystemdCgroupManager))
		})
		It("should be able to be set to cgroupfs", func() {
			// Given
			var err error
			sut, err = cgroupmanager.SetCgroupManager(cgroupmanager.CgroupfsCgroupManager)
			Expect(sut).To(Not(BeNil()))
			Expect(err).To(BeNil())
			// When
			name := sut.Name()

			// Then
			Expect(name).To(Equal(cgroupmanager.CgroupfsCgroupManager))
		})
		It("should be systemd when systemd v2", func() {
			// Given
			sut = new(cgroupmanager.Systemdv2Manager)
			// When
			name := sut.Name()

			// Then
			Expect(name).To(Equal(cgroupmanager.SystemdCgroupManager))
		})
	})
	t.Describe("IsSystemd", func() {
		It("should be systemd per default", func() {
			// Given
			// When
			res := sut.IsSystemd()

			// Then
			Expect(res).To(Equal(true))
		})
		It("should be able to be set to cgroupfs", func() {
			// Given
			var err error
			sut, err = cgroupmanager.SetCgroupManager(cgroupmanager.CgroupfsCgroupManager)
			Expect(sut).To(Not(BeNil()))
			Expect(err).To(BeNil())
			// When
			res := sut.IsSystemd()

			// Then
			Expect(res).To(Equal(false))
		})
		It("should be systemd when systemd v2", func() {
			// Given
			sut = new(cgroupmanager.Systemdv2Manager)
			// When
			res := sut.IsSystemd()

			// Then
			Expect(res).To(Equal(true))
		})
	})
	t.Describe("CgroupfsManager", func() {
		BeforeEach(func() {
			sut = new(cgroupmanager.CgroupfsManager)
		})
		t.Describe("GetContainerCgroupPath", func() {
			It("should contain default /crio", func() {
				// Given
				cid := "cid"
				// When
				cgroupPath := sut.GetContainerCgroupPath("", cid)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cid))
				Expect(cgroupPath).To(ContainSubstring("/crio"))
			})
			It("can override sandbox parent", func() {
				// Given
				cid := "cid"
				sbParent := "sandbox-parent"
				// When
				cgroupPath := sut.GetContainerCgroupPath(sbParent, cid)

				// Then
				Expect(cgroupPath).To(ContainSubstring(cid))
				Expect(cgroupPath).To(ContainSubstring(sbParent))
			})
		})
		t.Describe("GetSandboxCgroupPath", func() {
			It("should fail if sandbox parent has .slice", func() {
				// Given
				sbID := "sbid"
				sbParent := "sandbox-parent.slice"
				// When
				cgParent, cgPath, err := sut.GetSandboxCgroupPath(sbParent, sbID)

				// Then
				Expect(cgParent).To(BeEmpty())
				Expect(cgPath).To(BeEmpty())
				Expect(err).To(Not(BeNil()))
			})
			It("can override sandbox parent", func() {
				// Given
				sbID := "sbid"
				sbParent := "sandbox-parent"
				// When
				cgParent, cgPath, err := sut.GetSandboxCgroupPath(sbParent, sbID)

				// Then
				Expect(cgParent).To(Equal(sbParent))
				Expect(cgPath).To(ContainSubstring(sbParent))
				Expect(cgPath).To(ContainSubstring(sbID))
				Expect(err).To(BeNil())
			})
		})
		t.Describe("MoveConmonToCgroup", func() {
			It("should fail if invalid conmon cgroup", func() {
				// Given
				conmonCgroup := "notPodOrEmpty"
				// When
				cgPath := sut.MoveConmonToCgroup("", "", conmonCgroup, 0)

				// Then
				Expect(cgPath).To(BeEmpty())
			})
		})
	})
	t.Describe("Systemdv1Manager", func() {
		sharedSystemdManagerTests(new(cgroupmanager.Systemdv1Manager))
	})
	t.Describe("Systemdv2Manager", func() {
		sharedSystemdManagerTests(new(cgroupmanager.Systemdv2Manager))
	})
})

func sharedSystemdManagerTests(sut cgroupmanager.CgroupManager) {
	t.Describe("GetContainerCgroupPath", func() {
		It("should contain default system.slice", func() {
			// Given
			cid := "cid"
			// When
			cgroupPath := sut.GetContainerCgroupPath("", cid)

			// Then
			Expect(cgroupPath).To(ContainSubstring(cid))
			Expect(cgroupPath).To(ContainSubstring("system.slice"))
		})
		It("can override sandbox parent", func() {
			// Given
			cid := "cid"
			sbParent := "sandbox-parent"
			// When
			cgroupPath := sut.GetContainerCgroupPath(sbParent, cid)

			// Then
			Expect(cgroupPath).To(ContainSubstring(cid))
			Expect(cgroupPath).To(ContainSubstring(sbParent))
		})
	})
	t.Describe("GetSandboxCgroupPath", func() {
		It("should fail when parent too short", func() {
			// Given
			sbID := "sbid"
			sbParent := "slice"
			// When
			cgParent, cgPath, err := sut.GetSandboxCgroupPath(sbParent, sbID)

			// Then
			Expect(cgParent).To(BeEmpty())
			Expect(cgPath).To(BeEmpty())
			Expect(err).To(Not(BeNil()))
		})
		It("should fail when parent not slice", func() {
			// Given
			sbID := "sbid"
			sbParent := "systemd.invalid"
			// When
			cgParent, cgPath, err := sut.GetSandboxCgroupPath(sbParent, sbID)

			// Then
			Expect(cgParent).To(BeEmpty())
			Expect(cgPath).To(BeEmpty())
			Expect(err).To(Not(BeNil()))
		})
	})
}
