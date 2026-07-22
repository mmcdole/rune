//go:build luajit && windows

package luajit

// Windows has no system LuaJIT package; CI (and local Windows builds)
// compile a static libluajit.a from source into vendor_luajit/ next to
// this package -- see .github/actions/setup-luajit-windows. The
// archive keeps release binaries self-contained.

/*
#cgo CFLAGS: -I${SRCDIR}/vendor_luajit/include
#cgo LDFLAGS: ${SRCDIR}/vendor_luajit/lib/libluajit.a
*/
import "C"
