package worker

import (
	"os/exec"
	"syscall"
)

func configure(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} }
