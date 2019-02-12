// +build linux

package lvm

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/containers/storage/drivers/graphtest"
	"github.com/containers/storage/pkg/reexec"
)

var (
	counter = 0
	testdir = ""
)

func TestMain(m *testing.M) {
	if reexec.Init() {
		return
	}
	testdir, err := ioutil.TempDir("", "lvmtest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating temporary directory: %v\n", err)
		os.Exit(1)
	}
	status := m.Run()
	os.RemoveAll(testdir)
	os.Exit(status)
}

func getOptions() []string {
	counter++
	sparse := "false"
	if counter%2 != 0 {
		sparse = "true"
	}
	return []string{
		"lvm.loopback=" + filepath.Join(testdir, "loopbackfile"),
		"lvm.loopbacksize=128MB",
		"lvm.vg=lvm-test",
		"lvm.pool=layerpool",
		"lvm.sparse=" + sparse,
	}
}

func TestLVMChanges(t *testing.T) {
	graphtest.DriverTestChanges(t, "lvm", getOptions()...)
}

func TestLVMCreateBase(t *testing.T) {
	graphtest.DriverTestCreateBase(t, "lvm", getOptions()...)
}

func TestLVMCreateEmpty(t *testing.T) {
	graphtest.DriverTestCreateEmpty(t, "lvm", getOptions()...)
}

func TestLVMCreateSnap(t *testing.T) {
	graphtest.DriverTestCreateSnap(t, "lvm", getOptions()...)
}

func TestLVMDeepLayerRead(t *testing.T) {
	graphtest.DriverTestDeepLayerRead(t, 16, "lvm", getOptions()...)
}

func TestLVMDiffApply(t *testing.T) {
	graphtest.DriverTestDiffApply(t, 1024, "lvm", getOptions()...)
}
