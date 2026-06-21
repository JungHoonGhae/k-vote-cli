package nesdc

import (
	"fmt"
	"sort"
)

// Board describes an eGovFrame standard board on the NESDC portal. Every board
// shares the same list.do / view.do / FileDown.do plumbing; only the menu id,
// board id, and detail layout differ.
type Board struct {
	// Name is the short CLI identifier (e.g. "results").
	Name string
	// Title is the human-readable Korean label.
	Title string
	// BbsID is the eGovFrame board id (e.g. "B0000005").
	BbsID string
	// MenuNo is the portal menu number used as a query parameter.
	MenuNo string
	// Rich is true for boards whose detail page carries the full survey
	// metadata table (currently only the results board).
	Rich bool
}

// boards is the registry of known boards, keyed by Name.
var boards = map[string]Board{
	"results":    {Name: "results", Title: "여론조사결과 보기", BbsID: "B0000005", MenuNo: "200467", Rich: true},
	"data":       {Name: "data", Title: "여론조사결과 주요 데이터", BbsID: "B0000025", MenuNo: "200500"},
	"notices":    {Name: "notices", Title: "공지사항", BbsID: "B0000001", MenuNo: "200468"},
	"library":    {Name: "library", Title: "자료실", BbsID: "B0000002", MenuNo: "200470"},
	"actions":    {Name: "actions", Title: "선거여론조사기관 조치현황", BbsID: "B0000007", MenuNo: "200469"},
	"violations": {Name: "violations", Title: "유형별 위반사례", BbsID: "B0000045", MenuNo: "200480"},
}

// BoardByName returns the board registered under name.
func BoardByName(name string) (Board, error) {
	b, ok := boards[name]
	if !ok {
		return Board{}, fmt.Errorf("unknown board %q (known: %v)", name, BoardNames())
	}
	return b, nil
}

// BoardNames returns the sorted list of registered board names.
func BoardNames() []string {
	names := make([]string, 0, len(boards))
	for n := range boards {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Boards returns all registered boards, sorted by name.
func Boards() []Board {
	out := make([]Board, 0, len(boards))
	for _, n := range BoardNames() {
		out = append(out, boards[n])
	}
	return out
}
