//go:build !windows

package utils

import (
	"os"
	"syscall"
	"time"
)

func fileBirthTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	t := statBirthTime(st)
	if t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}
