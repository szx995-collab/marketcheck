//go:build !windows

package main

import "os/exec"

func hideCodexWindow(cmd *exec.Cmd) {}
