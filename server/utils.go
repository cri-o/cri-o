package server

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

func getGPRCVersion() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("Failed to recover the caller information.")
	}

	ocidRoot := filepath.Dir(filepath.Dir(file))
	p := filepath.Join(ocidRoot, "Godeps/Godeps.json")

	grepCmd := fmt.Sprintf(`grep -r "\"google.golang.org/grpc\"" %s -A 1 | grep "\"Rev\"" | cut -d: -f2 | tr -d ' "\n'`, p)

	out, err := execCmd("bash", "-c", grepCmd)
	if err != nil {
		return "", err
	}
	return out, nil
}
