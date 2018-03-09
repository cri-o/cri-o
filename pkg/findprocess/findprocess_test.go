package findprocess

import (
	"os/exec"
	"testing"
)

func TestFindEngine(t *testing.T) {
	cmd := exec.Command("sleep", "1")
	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	process, err := FindProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	err = process.Release()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	process, err = FindProcess(cmd.Process.Pid)
	if err == ErrNotFound {
		return
	}
	if err == nil {
		err = process.Release()
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("found the reaped process %d", cmd.Process.Pid)
	}
	t.Fatal(err)
}
