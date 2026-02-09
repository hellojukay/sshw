//go:build windows

package sshw

// FlushStdin clears any pending input from stdin (no-op on Windows)
func FlushStdin() {
	// Windows implementation if needed, currently no-op
}
