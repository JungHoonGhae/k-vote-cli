package nec

import "strings"

// Dimension is one source column to the left of the 선거인수 anchor, kept under
// its verbatim header label. The set/order varies by election type and year; we
// capture whatever is there rather than mapping each layout by hand.
type Dimension struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ElectionResult is the common, election-type-aware normalized row that both the
// CSV (총선·대선) and XLSX (지방선거 등) parsers converge to.
type ElectionResult struct {
	Race       string          `json:"race"`
	Dimensions []Dimension     `json:"dimensions"`
	VoteType   string          `json:"voteType"`
	Aggregate  bool            `json:"aggregate"`
	Electorate int             `json:"electorate"`
	Votes      int             `json:"votes"`
	Invalid    int             `json:"invalid"`
	Abstention int             `json:"abstention"`
	Candidates []CandidateVote `json:"candidates"`
}

// Dim returns the value of the dimension with the given label, or "".
func (e ElectionResult) Dim(label string) string {
	for _, d := range e.Dimensions {
		if d.Label == label {
			return d.Value
		}
	}
	return ""
}

// deriveVoteType maps a 구분 value to a vote-type label and the aggregate flag.
// aggregate reads only the source's own subtotal labels (합계/소계) — it embeds
// no interpretation of ours.
func deriveVoteType(gubun string) (voteType string, aggregate bool) {
	switch strings.ReplaceAll(gubun, " ", "") {
	case "합계", "소계":
		return "", true
	case "거소투표":
		return "거소", false
	case "관외사전투표":
		return "관외사전", false
	case "관내사전투표":
		return "관내사전", false
	default:
		return "본투표", false
	}
}
