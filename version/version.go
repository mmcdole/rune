// Package version is the single source of the client's identity.
// The network layer reports it over TTYPE/MNES, and the Lua engine
// exposes it as rune.version.
package version

const Name = "Rune"

// Number is stamped by goreleaser at release time
// (-X github.com/mmcdole/rune/version.Number={{.Version}}).
// The in-repo default marks untagged builds.
var Number = "0.1.0-dev"
