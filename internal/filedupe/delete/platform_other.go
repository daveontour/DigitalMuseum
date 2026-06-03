//go:build !windows

package delete

import (
	"os"
	"strings"
)

func removeFile(path string) error {
	return os.Remove(path)
}

func describeRemoveError(err error) string {
	return err.Error()
}

func accessDeniedHint() string {
	return ""
}

func sawAccessDenied(failed map[string]string) bool {
	return false
}
