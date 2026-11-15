//go:build windows

package commands

import "os"

// setReadTimeout is a no-op on Windows: the console in raw mode returns one
// Read per key event, so a lone ESC never blocks behind an escape sequence.
func setReadTimeout(fd int) {}

type rawByte struct {
	b  byte
	ok bool
}

// readByteTimeout returns one byte from a single console read.
func readByteTimeout() (rawByte, error) {
	var buf [1]byte
	n, err := os.Stdin.Read(buf[:])
	if n > 0 {
		return rawByte{buf[0], true}, nil
	}
	if err != nil {
		return rawByte{}, err
	}
	return rawByte{}, nil
}
