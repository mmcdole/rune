//go:build luajit && linux

package luajit

// Link the Debian/Ubuntu libluajit-5.1-dev static archive explicitly
// (-l: bypasses the .so preference) so release binaries run on systems
// without LuaJIT installed, and so trace mcode stays within branch
// range of our own text segment (see link_darwin_arm64.go).

/*
#cgo CFLAGS: -I/usr/include/luajit-2.1
#cgo LDFLAGS: -l:libluajit-5.1.a -lm -ldl
*/
import "C"
