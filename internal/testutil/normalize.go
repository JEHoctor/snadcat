// Package testutil holds helpers shared by the bash-parity tests.
package testutil

import "strings"

// Normalize maps snadcat's naming back onto sandcat's so Go output can be
// compared byte-for-byte with the bash oracle.
//
// The two tools deliberately differ in their on-disk and environment surface
// (.snadcat/, ~/.config/snadcat/, SNADCAT_*, snadcat-cache-*, the
// "# Snadcat" gitignore markers) so they can coexist on one machine. The
// shared templates never contain the string "snadcat", so every occurrence
// in Go output originates from the tool's own naming — which makes a global
// substitution sound rather than a heuristic.
func Normalize(s string) string {
	r := strings.NewReplacer(
		"SNADCAT_", "SANDCAT_",
		"Snadcat", "Sandcat",
		"snadcat", "sandcat",
	)
	return r.Replace(s)
}

// ToSnadcatEnv converts a SANDCAT_* variable name to the SNADCAT_* form the Go
// tool reads, so one test matrix can drive both sides.
func ToSnadcatEnv(name string) string {
	return strings.Replace(name, "SANDCAT_", "SNADCAT_", 1)
}
