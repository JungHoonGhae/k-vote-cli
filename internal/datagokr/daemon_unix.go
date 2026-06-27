//go:build !windows

package datagokr

import (
	"os/exec"
	"syscall"
)

// setDetached puts the launched browser in its own process group so it outlives
// kvote's exit (Unix).
func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
