package common

import (
	"os"
	"syscall"
	"unsafe"
)

type Tty struct{}

type winsize struct {
	ws_row    uint16
	ws_col    uint16
	ws_xpixel uint16
	ws_ypixel uint16
}

func (t *Tty) getWinSize(ws *winsize, fd uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func (t *Tty) setWinsize(fd uintptr, ws *winsize) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

func (t *Tty) GetSize(f *os.File) (rows, cols int, err error) {
	if f == nil {
		f = os.Stdin
	}
	var ws winsize
	err = t.getWinSize(&ws, f.Fd())
	return int(ws.ws_row), int(ws.ws_col), err
}

func (t *Tty) SetSize(f *os.File, width, height int) error {
	if f == nil {
		f = os.Stdin
	}
	ws := winsize{ws_row: uint16(width), ws_col: uint16(height), ws_xpixel: 0, ws_ypixel: 0}
	return t.setWinsize(f.Fd(), &ws)
}
