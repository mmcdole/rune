package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the rune configuration directory.
// Respects XDG_CONFIG_HOME on Unix, APPDATA on Windows.
func Dir() string {
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

// InitFile returns the path to init.lua
func InitFile() string {
	return filepath.Join(Dir(), "init.lua")
}
