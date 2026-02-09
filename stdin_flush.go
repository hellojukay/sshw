//go:build !windows

package main

import (
	"os"
	"syscall"
)

func flushStdin() {
	fd := int(os.Stdin.Fd())
	// Set non-blocking mode
	if err := syscall.SetNonblock(fd, true); err != nil {
		return
	}
	defer syscall.SetNonblock(fd, false)

	// Read and discard all data
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n <= 0 || err != nil {
			break
		}
	}
}
