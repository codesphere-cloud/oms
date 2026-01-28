package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ to the user's home directory
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
