package main

import (
	"os/exec"
	"syscall"
)

func hideCodexWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
