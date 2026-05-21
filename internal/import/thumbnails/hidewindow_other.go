//go:build !windows

package thumbnails

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
