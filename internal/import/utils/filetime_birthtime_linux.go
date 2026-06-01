//go:build linux

package utils

import (
	"syscall"
	"time"
)

func statBirthTime(st *syscall.Stat_t) time.Time {
	return time.Unix(st.Birthtim.Sec, st.Birthtim.Nsec)
}
