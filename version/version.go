// Package version is the single source of the client's identity.
// The network layer reports it over TTYPE/MNES, and the Lua engine
// exposes it as rune.version.
package version

const (
	Name   = "Rune"
	Number = "0.1.0"
)
