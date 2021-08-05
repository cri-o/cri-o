package dirnotifier_test

import (
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/dirnotifier"
	"github.com/fsnotify/fsnotify"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("DirectoryNotifier", func() {
	var testDir, testFile string
	BeforeEach(func() {
		testDir = t.MustTempDir("dirnotifier")
		testFile = filepath.Join(testDir, "file")
	})
	t.Describe("New", func() {
		It("should fail with invalid directory", func() {
			// When
			_, err := dirnotifier.New("doesnotexist", fsnotify.Create)
			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("Directory", func() {
		It("should be configured", func() {
			// When
			dn, err := dirnotifier.New(testDir, fsnotify.Create)
			Expect(err).To(BeNil())
			// Then
			Expect(dn.Directory()).To(Equal(testDir))
		})
	})
	t.Describe("NotifierForFile", func() {
		It("should fail on duplicate", func() {
			// Given
			dn, err := dirnotifier.New(testDir, fsnotify.Create)
			Expect(err).To(BeNil())
			// When
			_, err = dn.NotifierForFile(testFile)
			Expect(err).To(BeNil())
			// Then
			_, err = dn.NotifierForFile(testFile)
			Expect(err).NotTo(BeNil())
		})
		It("should notify on configured operation", func() {
			// Given
			dn, err := dirnotifier.New(testDir, fsnotify.Create)
			Expect(err).To(BeNil())
			// When
			notifierChan, err := dn.NotifierForFile(testFile)
			Expect(err).To(BeNil())
			f, err := os.Create(testFile)
			Expect(err).To(BeNil())
			f.Close()

			// Then
			<-notifierChan
		})
	})
})
