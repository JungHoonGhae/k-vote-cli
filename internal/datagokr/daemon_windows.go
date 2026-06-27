//go:build windows

package datagokr

import (
	"os/exec"
	"syscall"
)

// setDetached starts the browser in a new process group so it outlives kvote's
// exit (Windows). 0x00000200 = CREATE_NEW_PROCESS_GROUP.
func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
}
