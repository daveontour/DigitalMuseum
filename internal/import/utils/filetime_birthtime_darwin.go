//go:build darwin

package utils

import (
	"syscall"
	"time"
)

func statBirthTime(st *syscall.Stat_t) time.Time {
	return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}
