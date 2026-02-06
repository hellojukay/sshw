package sshw

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
)

// listRemote lists files in remote directory
func (s *SFTPShell) listRemote(args []string) {
	path := s.pwd
	if len(args) > 0 {
		path = s.resolvePath(args[0])
	}

	files, err := s.client.ReadDir(path)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	s.printFileList(files)
}

// listLocal lists files in local directory
func (s *SFTPShell) listLocal(args []string) {
	path := s.localPwd
	if len(args) > 0 {
		path = args[0]
	}

	files, err := os.ReadDir(path)
	if err != nil {
		fmt.Printf("Error reading local directory: %v\n", err)
		return
	}

	// Convert to []os.FileInfo for sorting
	var fileInfos []os.FileInfo
	for _, f := range files {
		info, err := f.Info()
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, info)
	}

	s.printFileList(fileInfos)
}

// printFileList prints file list in long format
func (s *SFTPShell) printFileList(files []os.FileInfo) {
	// Sort: directories first, then files
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir() && !files[j].IsDir() {
			return true
		}
		if !files[i].IsDir() && files[j].IsDir() {
			return false
		}
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		perms := file.Mode().String()
		size := file.Size()
		modTime := file.ModTime().Format("2006-01-02 15:04")

		if file.IsDir() {
			fmt.Printf("%s %10s %s %s/\n", perms, "", modTime, file.Name())
		} else {
			fmt.Printf("%s %10d %s %s\n", perms, size, modTime, file.Name())
		}
	}
}

// changeRemoteDir changes remote working directory
func (s *SFTPShell) changeRemoteDir(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cd <path>")
		return
	}

	newPath := s.resolvePath(args[0])

	// Verify directory exists
	info, err := s.client.Stat(newPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if !info.IsDir() {
		fmt.Printf("Error: %s is not a directory\n", newPath)
		return
	}

	s.pwd = newPath
}

// changeLocalDir changes local working directory
func (s *SFTPShell) changeLocalDir(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: lcd <path>")
		return
	}

	// Expand ~
	path := args[0]
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	// Resolve to absolute path
	var resolvedPath string
	if filepath.IsAbs(path) {
		resolvedPath = path
	} else {
		resolvedPath = filepath.Join(s.localPwd, path)
	}

	// Verify directory exists
	if _, err := os.Stat(resolvedPath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	s.localPwd = filepath.Clean(resolvedPath)
}

// downloadFile downloads file from remote to local
func (s *SFTPShell) downloadFile(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: get <remote-file> [local-file]")
		return
	}

	remotePath := s.resolvePath(args[0])
	localPath := ""

	if len(args) > 1 {
		localPath = args[1]
	} else {
		localPath = filepath.Base(remotePath)
	}

	// Get file size first for progress bar
	srcFile, err := s.client.Open(remotePath)
	if err != nil {
		fmt.Printf("Error opening remote file: %v\n", err)
		return
	}
	defer srcFile.Close()

	info, err := s.client.Stat(remotePath)
	var fileSize int64
	if err == nil {
		fileSize = info.Size()
	}

	// Create local file
	dstFile, err := os.Create(localPath)
	if err != nil {
		fmt.Printf("Error creating local file: %v\n", err)
		return
	}
	defer dstFile.Close()

	// Copy with progress
	bytesWritten, err := copyWithProgress(dstFile, srcFile, fileSize, fmt.Sprintf("Downloading %s", filepath.Base(remotePath)))
	if err != nil {
		fmt.Printf("\nError downloading file: %v\n", err)
		return
	}

	fmt.Printf("Download complete: %s (%.2f MB)\n", localPath, float64(bytesWritten)/1024/1024)
}

// uploadFile uploads file from local to remote
func (s *SFTPShell) uploadFile(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: put <local-file> [remote-file]")
		return
	}

	localPath := args[0]
	remotePath := ""

	if len(args) > 1 {
		remotePath = s.resolvePath(args[1])
	} else {
		remotePath = filepath.Join(s.pwd, filepath.Base(localPath))
	}

	// Get file size first for progress bar
	srcFile, err := os.Open(localPath)
	if err != nil {
		fmt.Printf("Error opening local file: %v\n", err)
		return
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	var fileSize int64
	if err == nil {
		fileSize = info.Size()
	}

	// Create remote file
	dstFile, err := s.client.Create(remotePath)
	if err != nil {
		fmt.Printf("Error creating remote file: %v\n", err)
		return
	}
	defer dstFile.Close()

	// Copy with progress
	bytesWritten, err := copyWithProgress(dstFile, srcFile, fileSize, fmt.Sprintf("Uploading %s", filepath.Base(localPath)))
	if err != nil {
		fmt.Printf("\nError uploading file: %v\n", err)
		return
	}

	fmt.Printf("Upload complete: %s (%.2f MB)\n", remotePath, float64(bytesWritten)/1024/1024)
}

// makeRemoteDir creates a remote directory
func (s *SFTPShell) makeRemoteDir(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mkdir <path>")
		return
	}

	path := s.resolvePath(args[0])
	err := s.client.Mkdir(path)
	if err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		return
	}

	fmt.Printf("Directory created: %s\n", path)
}

// makeLocalDir creates a local directory
func (s *SFTPShell) makeLocalDir(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: lmkdir <path>")
		return
	}

	path := args[0]
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.localPwd, path)
	}

	err := os.MkdirAll(path, 0755)
	if err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		return
	}

	fmt.Printf("Directory created: %s\n", path)
}

// removeRemote removes remote file/directory
func (s *SFTPShell) removeRemote(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: rm <path>")
		return
	}

	path := s.resolvePath(args[0])

	info, err := s.client.Stat(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if info.IsDir() {
		err = s.client.RemoveDirectory(path)
	} else {
		err = s.client.Remove(path)
	}

	if err != nil {
		fmt.Printf("Error removing: %v\n", err)
		return
	}

	fmt.Printf("Removed: %s\n", path)
}

// removeLocal removes local file/directory
func (s *SFTPShell) removeLocal(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: lrm <path>")
		return
	}

	path := args[0]
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.localPwd, path)
	}

	err := os.RemoveAll(path)
	if err != nil {
		fmt.Printf("Error removing: %v\n", err)
		return
	}

	fmt.Printf("Removed: %s\n", path)
}

// moveRemote moves/renotes remote file
func (s *SFTPShell) moveRemote(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: mv <source> <destination>")
		return
	}

	oldPath := s.resolvePath(args[0])
	newPath := s.resolvePath(args[1])

	err := s.client.Rename(oldPath, newPath)
	if err != nil {
		fmt.Printf("Error moving: %v\n", err)
		return
	}

	fmt.Printf("Moved: %s -> %s\n", oldPath, newPath)
}

// moveLocal moves/renotes local file
func (s *SFTPShell) moveLocal(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: lmv <source> <destination>")
		return
	}

	oldPath := args[0]
	newPath := args[1]

	if !filepath.IsAbs(oldPath) {
		oldPath = filepath.Join(s.localPwd, oldPath)
	}

	if !filepath.IsAbs(newPath) {
		newPath = filepath.Join(s.localPwd, newPath)
	}

	err := os.Rename(oldPath, newPath)
	if err != nil {
		fmt.Printf("Error moving: %v\n", err)
		return
	}

	fmt.Printf("Moved: %s -> %s\n", oldPath, newPath)
}

// resolvePath resolves relative paths against current remote directory
func (s *SFTPShell) resolvePath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	if path == "~" {
		return "/home/" + s.node.user()
	}
	if strings.HasPrefix(path, "~/") {
		return "/home/" + s.node.user() + path[2:]
	}
	return filepath.Join(s.pwd, path)
}

// copyWithProgress copies data with progress display using progressbar library
func copyWithProgress(dst io.Writer, src io.Reader, total int64, description string) (int64, error) {
	bar := progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("B/s"),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
	)

	// Use io.Copy with the progress bar as a writer that wraps dst
	written, err := io.Copy(io.MultiWriter(dst, bar), src)
	if err != nil {
		return written, err
	}

	bar.Finish()
	return written, nil
}
