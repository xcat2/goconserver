package common

import (
	"golang.org/x/sys/unix"
	"syscall"
)

func Fcntl(fd, cmd int, arg int) error {
	_, _, e := syscall.Syscall(unix.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}
