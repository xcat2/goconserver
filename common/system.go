package common

import (
	"syscall"
)

func Fcntl(fd, cmd int, arg int) error {
	_, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}
