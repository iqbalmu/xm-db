//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package commands

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// setReadTimeout reconfigures the raw tty so reads give up after 100ms
// without input; that is how a lone ESC is told apart from the escape
// sequences terminals send for arrow keys.
func setReadTimeout(fd int) {
	tio, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	tio.Cc[unix.VMIN] = 0
	tio.Cc[unix.VTIME] = 1 // 100ms
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, tio)
}

type rawByte struct {
	b  byte
	ok bool
}

// readByteTimeout returns one byte, or ok=false after the 100ms window.
func readByteTimeout() (rawByte, error) {
	var buf [1]byte
	n, err := os.Stdin.Read(buf[:])
	if n > 0 {
		return rawByte{buf[0], true}, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return rawByte{}, err
	}
	return rawByte{}, nil // timeout
}
