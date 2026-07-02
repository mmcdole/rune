package text

// Degraded-path styling. Presentation belongs to Lua (rune.style);
// these helpers exist for the few messages Go must print itself -
// boot failures, degraded-mode warnings, the local-echo prefix - so
// raw escape codes live in exactly one Go file.

// Red wraps s in red ANSI codes. Used for Go-side error fallbacks.
func Red(s string) string { return "\x1b[31m" + s + "\x1b[0m" }

// Green wraps s in green ANSI codes. Used for the local-echo prefix.
func Green(s string) string { return "\x1b[32m" + s + "\x1b[0m" }
