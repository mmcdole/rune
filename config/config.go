package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the rune configuration directory. RUNE_CONFIG_DIR takes
// precedence over the platform default.
func Dir() string {
	return ResolveDir("")
}

// ResolveDir applies the precedence for Rune's configuration
// directory: a non-empty CLI value wins, followed by RUNE_CONFIG_DIR,
// then the platform default.
func ResolveDir(cliDir string) string {
	if cliDir != "" {
		return cliDir
	}
	if envDir := os.Getenv("RUNE_CONFIG_DIR"); envDir != "" {
		return envDir
	}

	var base string

	if runtime.GOOS == "windows" {
		base = os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	} else {
		base = os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".config")
		}
	}

	return filepath.Join(base, "rune")
}
