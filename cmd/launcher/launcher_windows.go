package main

import (
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow  = 0x08000000
	detachedProcess = 0x00000008
)

// hiddenProcAttr suppresses any console window for short-lived helper
// commands (taskkill, pg_ctl, initdb, cmd /c start).
func hiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}

// detachedProcAttr makes the child process independent of the parent's
// process group and suppresses any console window. Used for
// digitalmuseum.exe so it can outlive the launcher if needed.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNoWindow,
	}
}

// alreadyRunning creates a named Windows Mutex. If the mutex already exists
// (ERROR_ALREADY_EXISTS), another launcher instance is running.
func alreadyRunning() bool {
	name, _ := windows.UTF16PtrFromString("DigitalMuseumLauncherMutex")
	_, err := windows.CreateMutex(nil, false, name)
	return err == windows.ERROR_ALREADY_EXISTS
}
