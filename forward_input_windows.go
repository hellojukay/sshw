//go:build windows

package sshw

import (
    "io"
    "os"
)

// forwardInput on Windows falls back to a simple io.Copy
// which is compatible with console input. The done channel
// is not used due to lack of a portable non-blocking console read.
func forwardInput(fd int, stdinPipe io.WriteCloser, done <-chan struct{}) {
    go func() {
        _, _ = io.Copy(stdinPipe, os.Stdin)
    }()
}

