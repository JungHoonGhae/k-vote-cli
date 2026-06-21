// Command kvote is an unofficial CLI for Korean election data: it collects
// public records from NESDC (선거여론조사심의위원회) and, where keyless public
// access exists, NEC (중앙선거관리위원회).
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
