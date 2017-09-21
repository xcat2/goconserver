package common

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Tty struct{}

func (*Tty) size() (int, int, error) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	parts := strings.Split(string(out), " ")
	if len(parts) != 2 {
		return 0, 0, errors.New("The output of 'stty size' command is not supported")
	}
	x, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errors.New("The output of 'stty size' command is not supported")
	}
	y, err := strconv.Atoi(strings.Replace(parts[1], "\n", "", 1))
	if err != nil {
		return 0, 0, errors.New("The output of 'stty size' command is not supported")
	}
	return int(x), int(y), err
}

func (t *Tty) Width() (int, error) {
	width, _, err := t.size()
	if err != nil {
		return 0, err
	}
	return width, err
}

// Height returns the height of the terminal.
func (t *Tty) Height() (int, error) {
	_, height, err := t.size()
	if err != nil {
		return 0, err
	}
	return height, err
}
