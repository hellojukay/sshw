//go:build !windows

package sshw

import (
	"io"
	"syscall"
	"time"
)

// forwardInput on Unix-like systems uses non-blocking reads to allow
// graceful termination when the SSH session ends.
func forwardInput(fd int, stdinPipe io.WriteCloser, done <-chan struct{}) {
	// enable non-blocking mode for stdin
	_ = syscall.SetNonblock(fd, true)
	defer syscall.SetNonblock(fd, false)

	buf := make([]byte, 4096)
	for {
		select {
		case <-done:
			return
		default:
		}

		n, rerr := syscall.Read(fd, buf)
		if n > 0 {
			if _, werr := stdinPipe.Write(buf[:n]); werr != nil {
				return
			}
		}
		if rerr != nil {
			// expected when no input available
			if rerr == syscall.EAGAIN || rerr == syscall.EWOULDBLOCK || rerr == syscall.EINTR {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			// other errors: exit
			return
		}
	}
}
