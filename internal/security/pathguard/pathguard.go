package pathguard

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func CanonicalDir(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("empty root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("root must not be a symlink")
	}
	return abs, nil
}

func ValidateRelative(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", nil
	}
	if strings.Contains(s, "\\") {
		return "", fmt.Errorf("path contains backslashes")
	}
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	for _, part := range strings.Split(s, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("path traversal detected")
		}
	}
	clean := path.Clean(s)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

func IsHiddenPath(rel string) bool {
	if rel == "" {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}
