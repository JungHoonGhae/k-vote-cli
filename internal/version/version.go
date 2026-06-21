// Package version holds build metadata injected via -ldflags.
package version

import "fmt"

var (
	// Version is the semantic version, set at build time.
	Version = "dev"
	// Commit is the short git SHA, set at build time.
	Commit = "unknown"
	// Date is the build timestamp (RFC3339), set at build time.
	Date = "unknown"
)

// String renders a human-readable version line.
func String() string {
	return fmt.Sprintf("kvote %s (commit %s, built %s)", Version, Commit, Date)
}
