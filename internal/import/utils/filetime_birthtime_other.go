//go:build !linux && !darwin && !windows

package utils

import (
	"syscall"
	"time"
)

func statBirthTime(st *syscall.Stat_t) time.Time {
	return time.Time{}
}
