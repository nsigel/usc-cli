//go:build !windows

package libcal

import (
	"os"
	"syscall"
)

func lockCheckout(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
