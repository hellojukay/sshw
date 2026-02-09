package sshw

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/atrox/homedir"
)

func expandHomePath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		return homedir.Expand(p)
	}
	return p, nil
}

func resolveLocalPath(base, input string) (string, error) {
	if input == "" {
		return filepath.Clean(base), nil
	}

	expanded, err := expandHomePath(input)
	if err != nil {
		return "", err
	}

	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}

	return filepath.Clean(filepath.Join(base, expanded)), nil
}

func resolveRemotePath(pwd, user, input string) string {
	if input == "" {
		return pwd
	}
	if strings.HasPrefix(input, "/") {
		return input
	}
	if input == "~" {
		return path.Join("/home", user)
	}
	if strings.HasPrefix(input, "~/") {
		return path.Join("/home", user, input[2:])
	}
	return path.Join(pwd, input)
}
