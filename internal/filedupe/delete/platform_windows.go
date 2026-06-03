//go:build windows

package delete

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func removeFile(path string) error {
	return os.Remove(path)
}

func describeRemoveError(err error) string {
	if isAccessDenied(err) {
		return "access denied (blocked by Windows Controlled Folder Access)"
	}
	return err.Error()
}

func accessDeniedHint() string {
	exe, err := os.Executable()
	if err != nil {
		exe = "digitalmuseum.exe"
	}

	hint := fmt.Sprintf(`Windows blocked the delete (Controlled Folder Access).

Add Digital Museum to the allowed apps list:
1. Open Windows Security
2. Virus & threat protection → Manage ransomware protection
3. Controlled folder access → Allow an app through Controlled folder access
4. Add this program:
   %s`, exe)

	if strings.Contains(strings.ToLower(exe), `\go-build\`) {
		hint += `

Note: "go run" uses a temporary executable that changes each build.
Run "make build-exe" and allow bin/digitalmuseum.exe instead.`
	}

	return hint
}

func isAccessDenied(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == syscall.ERROR_ACCESS_DENIED {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "access is denied")
}

func sawAccessDenied(failed map[string]string) bool {
	for _, msg := range failed {
		if strings.Contains(strings.ToLower(msg), "access denied") ||
			strings.Contains(strings.ToLower(msg), "controlled folder access") {
			return true
		}
	}
	return false
}
