//go:build luajit && !linux && !windows && !(darwin && arm64)

package luajit

/*
#cgo pkg-config: luajit
*/
import "C"
