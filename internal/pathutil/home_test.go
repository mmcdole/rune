package pathutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("home", home)
	tests := []struct{ path, want string }{
		{"", ""},
		{"~", home},
		{"~/", home},
		{"~/dir/file", filepath.Join(home, "dir", "file")},
		{"~user", "~user"},
		{"~user/x", "~user/x"},
		{"~other/x", "~other/x"},
		{"relative/file", "relative/file"},
		{"dir/~/file", "dir/~/file"},
		{home, home},
	}
	backslash := `~\dir\file`
	wantBackslash := backslash
	if runtime.GOOS == "windows" {
		wantBackslash = filepath.Join(home, "dir", "file")
	}
	tests = append(tests, struct{ path, want string }{backslash, wantBackslash})
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := ExpandHome(tc.path); got != tc.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestExpandHomeWithoutHome(t *testing.T) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		t.Skip("platform provides a default home")
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	for _, path := range []string{"~", "~/x", "~user/x"} {
		if got := ExpandHome(path); got != path {
			t.Errorf("ExpandHome(%q) = %q without a home", path, got)
		}
	}
}
