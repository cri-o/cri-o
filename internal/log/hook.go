package log

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

func RemoveHook(logger *logrus.Logger, name string) {
	filteredHooks := make(logrus.LevelHooks)

	for level, hooks := range logger.Hooks {
		for _, hook := range hooks {
			if fmt.Sprintf("%T", hook) != "*log."+name {
				filteredHooks[level] = append(filteredHooks[level], hook)
			}
		}
	}

	logger.ReplaceHooks(filteredHooks)
}
