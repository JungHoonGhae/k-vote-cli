package nec

import (
	"strconv"
	"strings"
)

// itoa keeps call sites terse.
func itoa(i int) string { return strconv.Itoa(i) }

// cleanText collapses whitespace (including the non-breaking spaces the portal
// emits) and trims the result.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	return strings.Join(strings.Fields(s), " ")
}
