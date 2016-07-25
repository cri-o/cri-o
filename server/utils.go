package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func removeFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func parseDNSOptions(servers, searches []string, path string) error {
	nServers := len(servers)
	nSearches := len(searches)
	if nServers == 0 && nSearches == 0 {
		return copyFile("/etc/resolv.conf", path)
	}

	if nSearches > maxDNSSearches {
		return fmt.Errorf("DNSOption.Searches has more than 6 domains")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if nSearches > 0 {
		data := fmt.Sprintf("search %s\n", strings.Join(searches, " "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	if nServers > 0 {
		data := fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
		_, err = f.Write([]byte(data))
		if err != nil {
			return err
		}
	}

	return nil
}
