// Package paths resolves the filesystem paths users give Rune: the config
// directory and script or log paths that may start with ~.
package paths

import (
	"os"
	"path/filepath"
)

// ExpandHome expands ~ and paths beginning with ~/ to the current user's
// home directory. It also accepts the native path separator. Named-user paths
// such as ~user/x are left unchanged, as are paths when no home is available.
func ExpandHome(path string) string {
	if path != "~" && (len(path) < 2 || path[0] != '~' || !os.IsPathSeparator(path[1])) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// Resolve expands a leading ~ and returns the absolute path.
func Resolve(path string) (string, error) {
	return filepath.Abs(ExpandHome(path))
}
