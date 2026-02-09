package sshw

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

type terminalManager struct {
	stdinFD  int
	stdoutFD int
}

func newTerminalManager() *terminalManager {
	return &terminalManager{
		stdinFD:  int(os.Stdin.Fd()),
		stdoutFD: int(os.Stdout.Fd()),
	}
}

func (t *terminalManager) makeRaw() (func(), error) {
	state, err := terminal.MakeRaw(t.stdinFD)
	if err != nil {
		return nil, err
	}
	return func() {
		_ = terminal.Restore(t.stdinFD, state)
	}, nil
}

func (t *terminalManager) size() (int, int, error) {
	return terminal.GetSize(t.stdoutFD)
}

func (t *terminalManager) startResizeMonitor(session *ssh.Session, initialW, initialH int, stop <-chan struct{}) {
	go func() {
		ow, oh := initialW, initialH
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				cw, ch, err := t.size()
				if err != nil {
					return
				}
				if cw != ow || ch != oh {
					if err := session.WindowChange(ch, cw); err != nil {
						return
					}
					ow, oh = cw, ch
				}
			}
		}
	}()
}

func readPassword() (string, error) {
	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(b), nil
}
