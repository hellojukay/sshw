package sshw

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/sftp"
)

// SFTPShell manages the interactive SFTP session
type SFTPShell struct {
	client   *sftp.Client
	node     *Node
	pwd      string // Current working directory on remote
	localPwd string // Current working directory on local
	reader   *bufio.Reader
	running  bool
}

// NewSFTPShell creates a new SFTP shell instance
func NewSFTPShell(client *sftp.Client, node *Node) *SFTPShell {
	// Get initial remote working directory
	pwd, err := client.Getwd()
	if err != nil {
		pwd = "."
	}

	// Get initial local working directory
	localPwd, _ := os.Getwd()

	return &SFTPShell{
		client:   client,
		node:     node,
		pwd:      pwd,
		localPwd: localPwd,
		reader:   bufio.NewReader(os.Stdin),
		running:  true,
	}
}

// Run starts the interactive SFTP shell
func (s *SFTPShell) Run() {
	fmt.Printf("Connected to %s@%s\n", s.node.user(), s.node.Host)
	fmt.Printf("Type 'help' for available commands, 'exit' or 'quit' to disconnect\n\n")

	for s.running {
		prompt := fmt.Sprintf("sftp %s:%s> ", s.node.Host, s.pwd)
		fmt.Print(prompt)

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nConnection closed.")
				return
			}
			continue
		}

		cmd := strings.TrimSpace(line)
		if cmd == "" {
			continue
		}

		s.executeCommand(cmd)
	}
}

// executeCommand parses and executes SFTP commands
func (s *SFTPShell) executeCommand(cmdLine string) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help", "?":
		s.showHelp()
	case "ls", "ll":
		s.listRemote(args)
	case "cd":
		s.changeRemoteDir(args)
	case "pwd":
		fmt.Println(s.pwd)
	case "lpwd":
		fmt.Println(s.localPwd)
	case "lcd":
		s.changeLocalDir(args)
	case "lls":
		s.listLocal(args)
	case "get":
		s.downloadFile(args)
	case "put":
		s.uploadFile(args)
	case "mkdir":
		s.makeRemoteDir(args)
	case "lmkdir":
		s.makeLocalDir(args)
	case "rm":
		s.removeRemote(args)
	case "lrm":
		s.removeLocal(args)
	case "mv":
		s.moveRemote(args)
	case "lmv":
		s.moveLocal(args)
	case "exit", "quit", "bye":
		s.running = false
		fmt.Println("Goodbye!")
	default:
		fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
	}
}

// showHelp displays available commands
func (s *SFTPShell) showHelp() {
	helpText := `
Available SFTP Commands:

Remote Operations:
  ls [path]           - List remote files (optional path)
  cd <path>           - Change remote directory
  pwd                 - Print remote working directory
  mkdir <path>        - Create remote directory
  rm <file>           - Remove remote file
  mv <src> <dst>      - Move/rename remote file

Local Operations:
  lls [path]          - List local files
  lcd <path>          - Change local directory
  lpwd                - Print local working directory
  lmkdir <path>       - Create local directory
  lrm <file>          - Remove local file
  lmv <src> <dst>     - Move/rename local file

File Transfer:
  get <remote> [local]  - Download file from remote
  put <local> [remote]  - Upload file to remote

General:
  help, ?             - Show this help message
  exit, quit, bye     - Exit SFTP session
`
	fmt.Println(helpText)
}
