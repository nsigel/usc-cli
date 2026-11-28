//go:build !windows

package cli

import "syscall"

func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
