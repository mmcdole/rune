//go:build luajit && !(darwin && arm64)

package luajit

/*
#cgo pkg-config: luajit
*/
import "C"
