package sshw

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

// listRemote lists files in remote directory
func (s *SFTPShell) listRemote(args []string) {
	path := s.pwd
	if len(args) > 0 {
		path = resolveRemotePath(s.pwd, s.node.user(), args[0])
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
		resolved, err := resolveLocalPath(s.localPwd, args[0])
		if err != nil {
			fmt.Printf("Error resolving local path: %v\n", err)
			return
		}
		path = resolved
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

	newPath := resolveRemotePath(s.pwd, s.node.user(), args[0])

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

	resolvedPath, err := resolveLocalPath(s.localPwd, args[0])
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
	}

	// Verify directory exists
	if _, err := os.Stat(resolvedPath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	s.localPwd = resolvedPath
}

// downloadFile downloads file from remote to local
func (s *SFTPShell) downloadFile(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: get <remote-file> [local-file]")
		return
	}

	remotePath := resolveRemotePath(s.pwd, s.node.user(), args[0])
	localPath := ""

	if len(args) > 1 {
		var err error
		localPath, err = resolveLocalPath(s.localPwd, args[1])
		if err != nil {
			fmt.Printf("Error resolving local path: %v\n", err)
			return
		}
	} else {
		// 默认保存到 localPwd
		localPath = filepath.Join(s.localPwd, filepath.Base(remotePath))
	}

	// Get file size first
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

	// Wrap dstFile with progress tracking
	progressDst := &progressWriter{
		writer:      dstFile,
		total:       fileSize,
		description: fmt.Sprintf("Downloading %s", filepath.Base(remotePath)),
	}

	// Use WriteTo for optimized concurrent reads from remote server
	bytesWritten, err := srcFile.WriteTo(progressDst)
	if err != nil {
		// Truncate local file to avoid data holes when transfer fails
		if info, statErr := dstFile.Stat(); statErr == nil {
			_ = dstFile.Truncate(info.Size())
		}
		fmt.Printf("\nError downloading file: %v\n", err)
		return
	}

	fmt.Fprint(os.Stderr, "\n")
	fmt.Printf("Download complete: %s (%.2f MB)\n", localPath, float64(bytesWritten)/1024/1024)
}

// uploadFile uploads file from local to remote
func (s *SFTPShell) uploadFile(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: put <local-file> [remote-file]")
		return
	}

	localPath := args[0]
	localPath, err := resolveLocalPath(s.localPwd, localPath)
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
	}
	remotePath := ""

	if len(args) > 1 {
		remotePath = resolveRemotePath(s.pwd, s.node.user(), args[1])
	} else {
		remotePath = path.Join(s.pwd, filepath.Base(localPath))
	}

	// Get file size first
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

	// Wrap srcFile with progress tracking
	// progressReader implements Size() which enables sftp.File.ReadFrom to use concurrent writes
	progressSrc := &progressReader{
		reader:      srcFile,
		total:       fileSize,
		description: fmt.Sprintf("Uploading %s", filepath.Base(localPath)),
	}

	// Use ReadFrom for optimized concurrent writes to remote server
	bytesWritten, err := dstFile.ReadFrom(progressSrc)
	if err != nil {
		// Truncate remote file to avoid data holes when concurrent write fails
		if info, statErr := dstFile.Stat(); statErr == nil {
			_ = dstFile.Truncate(info.Size())
		}
		fmt.Printf("\nError uploading file: %v\n", err)
		return
	}

	fmt.Fprint(os.Stderr, "\n")
	fmt.Printf("Upload complete: %s (%.2f MB)\n", remotePath, float64(bytesWritten)/1024/1024)
}

// makeRemoteDir creates a remote directory
func (s *SFTPShell) makeRemoteDir(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: mkdir <path>")
		return
	}

	path := resolveRemotePath(s.pwd, s.node.user(), args[0])
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

	path, err := resolveLocalPath(s.localPwd, args[0])
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
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

	path := resolveRemotePath(s.pwd, s.node.user(), args[0])

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

	path, err := resolveLocalPath(s.localPwd, args[0])
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
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

	oldPath := resolveRemotePath(s.pwd, s.node.user(), args[0])
	newPath := resolveRemotePath(s.pwd, s.node.user(), args[1])

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

	oldPath, err := resolveLocalPath(s.localPwd, args[0])
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
	}

	newPath, err := resolveLocalPath(s.localPwd, args[1])
	if err != nil {
		fmt.Printf("Error resolving local path: %v\n", err)
		return
	}

	err := os.Rename(oldPath, newPath)
	if err != nil {
		fmt.Printf("Error moving: %v\n", err)
		return
	}

	fmt.Printf("Moved: %s -> %s\n", oldPath, newPath)
}

// progressReader wraps an io.Reader to track progress for uploads.
// It implements Size() to enable concurrent writes in sftp.File.ReadFrom.
type progressReader struct {
	reader      io.Reader
	total       int64
	written     int64
	description string
	bar         *progressbar.ProgressBar
	mu          sync.Mutex
	once        sync.Once
}

// Size returns the total size of the data to be read.
// This enables sftp.File.ReadFrom to use concurrent writes.
func (pr *progressReader) Size() int64 {
	return pr.total
}

func (pr *progressReader) Read(p []byte) (int, error) {
	pr.once.Do(func() {
		pr.bar = progressbar.NewOptions64(
			pr.total,
			progressbar.OptionSetDescription(pr.description),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowCount(),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(40),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	})

	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.mu.Lock()
		pr.written += int64(n)
		pr.mu.Unlock()
		if pr.bar != nil {
			_ = pr.bar.Add(n)
		}
	}
	return n, err
}

// progressWriter wraps an io.Writer to track progress for downloads.
type progressWriter struct {
	writer      io.Writer
	total       int64
	written     int64
	description string
	bar         *progressbar.ProgressBar
	mu          sync.Mutex
	once        sync.Once
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	pw.once.Do(func() {
		pw.bar = progressbar.NewOptions64(
			pw.total,
			progressbar.OptionSetDescription(pw.description),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowCount(),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(40),
			progressbar.OptionThrottle(100*time.Millisecond),
		)
	})

	n, err := pw.writer.Write(p)
	if n > 0 {
		pw.mu.Lock()
		pw.written += int64(n)
		pw.mu.Unlock()
		if pw.bar != nil {
			_ = pw.bar.Add(n)
		}
	}
	return n, err
}
