//go:build !windows

package sshw

import (
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

// forwardInput on Unix-like systems uses poll to wait for input
// readiness and performs blocking reads. This avoids leaving stdin
// in non-blocking mode while still allowing timely shutdown.
func forwardInput(fd int, stdinPipe io.WriteCloser, done <-chan struct{}) {
	buf := make([]byte, 4096)
	p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		select {
		case <-done:
			return
		default:
		}

		// Wait up to 100ms for input readiness
		_, perr := unix.Poll(p, 100)
		if perr != nil {
			// Interrupted or error, retry unless fatal
			if perr == syscall.EINTR {
				continue
			}
			return
		}
		if p[0].Revents&unix.POLLIN == 0 {
			// No input ready, loop
			continue
		}

		n, rerr := syscall.Read(fd, buf)
		if n > 0 {
			if _, werr := stdinPipe.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			if rerr == syscall.EINTR {
				continue
			}
			return
		}
	}
}
