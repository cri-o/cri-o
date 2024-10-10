package linklogs

import (
	"fmt"
)

func mountLogPath(podLogsPath, emptyDirLoggingVolumePath string) error {
	return fmt.Errorf("linklogs unsupported on freebsd")
}

func unmountLogPath(emptyDirLoggingVolumePath string) error {
	return fmt.Errorf("linklogs unsupported on freebsd")
}
