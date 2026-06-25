package nesdc

import (
	"sort"
	"strings"
)

// SearchField maps a friendly CLI name to the results board's searchCnd code.
// These select which column a keyword search matches against.
var SearchField = map[string]string{
	"regno":  "5",  // 등록번호
	"agency": "1",  // 조사기관명
	"client": "2",  // 조사의뢰자
	"method": "6",  // 조사방법
	"frame":  "11", // 표본 추출틀
	"name":   "3",  // 여론조사명칭(지역)
	"sido":   "4",  // 시·도
}

// DateField maps a friendly CLI name to the results board's searchTime code,
// naming which date the --from/--to range filters on.
var DateField = map[string]string{
	"registered": "1", // 등록일
	"published":  "2", // 최초공표일
	"surveyed":   "3", // 조사일시
}

// ResolveSearchField maps a --field value (friendly name or raw code) to the
// searchCnd code.
func ResolveSearchField(v string) string { return resolveChoice(SearchField, v) }

// ResolveDateField maps a --date-field value (friendly name or raw code) to the
// searchTime code.
func ResolveDateField(v string) string { return resolveChoice(DateField, v) }

// resolveChoice returns the code for a friendly name, or the input unchanged if
// it is already a raw code (or unknown). This lets users pass either form.
func resolveChoice(m map[string]string, v string) string {
	if v == "" {
		return ""
	}
	if code, ok := m[strings.ToLower(v)]; ok {
		return code
	}
	return v
}

// SortedKeys returns a filter map's friendly names in a stable order, for help
// text and the `nesdc fields` command.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
