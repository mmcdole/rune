//go:build luajit && darwin && arm64

package luajit

// LuaJIT must be linked statically here: trace machine code has to sit
// within the arm64 +-128MB branch range of the VM code, and with the
// dylib mapped into the crowded dyld region that window is often full,
// making JIT compilation fail probabilistically per process. Static
// linking anchors the VM in our own text segment, where the loader
// constructor in shim.c reserves branch-range address space before the
// Go runtime can claim it.

/*
#cgo CFLAGS: -I/opt/homebrew/include/luajit-2.1
#cgo LDFLAGS: /opt/homebrew/lib/libluajit-5.1.a
*/
import "C"
