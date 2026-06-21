package nec

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

// CandidateVote is one candidate's tally within a polling unit.
type CandidateVote struct {
	Party string `json:"party"`
	Name  string `json:"name"`
	Votes int    `json:"votes"`
}

// ResultRecord is the normalized 개표결과 of a single polling unit (시도 →
// 선거구 → 법정읍면동 → 투표구), with turnout metrics and per-candidate votes.
// The source CSV is long-format (one row per metric/candidate); this collapses
// each polling unit into one record.
type ResultRecord struct {
	Sido       string          `json:"sido"`
	District   string          `json:"district"`
	Town       string          `json:"town"`
	Booth      string          `json:"booth,omitempty"`
	Electorate int             `json:"electorate"`
	Votes      int             `json:"votes"`
	Invalid    int             `json:"invalid"`
	Abstention int             `json:"abstention"`
	Candidates []CandidateVote `json:"candidates"`
}

// metricLabels maps a whitespace-stripped 후보자-column label to the turnout
// field it represents. Anything not listed here is treated as a candidate.
var metricLabels = map[string]string{
	"선거인수":  "electorate",
	"투표수":   "votes",
	"무효투표수": "invalid",
	"기권자수":  "abstention",
}

// resultHeader is the expected CSV column order.
var resultHeader = []string{"시도명", "선거구명", "법정읍면동명", "투표구명", "후보자", "득표수"}

// ParseResults decodes a NEC 개표결과 file (UTF-8 or EUC-KR/CP949) and collapses
// its long-format rows into one ResultRecord per polling unit. Rows are already
// grouped by polling unit in the source, so a key change flushes a record.
func ParseResults(raw []byte) ([]ResultRecord, error) {
	r := csv.NewReader(strings.NewReader(decodeKorean(raw)))
	r.FieldsPerRecord = -1 // tolerate ragged rows

	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if len(head) < len(resultHeader) || strings.TrimSpace(head[4]) != "후보자" {
		return nil, fmt.Errorf("unexpected columns %v; this dataset may be XLSX or a different layout — use `nec pull` for the raw file", head)
	}

	var out []ResultRecord
	var cur *ResultRecord
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) < 6 {
			continue
		}
		sido, dist, town, booth := strings.TrimSpace(row[0]), strings.TrimSpace(row[1]), strings.TrimSpace(row[2]), strings.TrimSpace(row[3])
		label := strings.TrimSpace(row[4])
		n := atoiLoose(row[5])

		// Each polling unit's block begins with a 선거인수 row, so that label —
		// not the (시도,선거구,읍면동,투표구) tuple — delimits records. The tuple
		// is not unique: combined districts and split early-voting booths reuse
		// the same names, which key-based grouping would silently merge.
		if strings.ReplaceAll(label, " ", "") == "선거인수" || cur == nil {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &ResultRecord{Sido: sido, District: dist, Town: town, Booth: booth}
		}

		if field, ok := metricLabels[strings.ReplaceAll(label, " ", "")]; ok {
			switch field {
			case "electorate":
				cur.Electorate = n
			case "votes":
				cur.Votes = n
			case "invalid":
				cur.Invalid = n
			case "abstention":
				cur.Abstention = n
			}
			continue
		}
		party, name := splitCandidate(label)
		cur.Candidates = append(cur.Candidates, CandidateVote{Party: party, Name: name, Votes: n})
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, nil
}

// splitCandidate splits a "정당 후보자명" label into party and name on the first
// space (independents are "무소속 이름"). A label without a space is returned as
// the name with an empty party.
func splitCandidate(label string) (party, name string) {
	if i := strings.IndexByte(label, ' '); i >= 0 {
		return label[:i], strings.TrimSpace(label[i+1:])
	}
	return "", label
}

// atoiLoose parses an integer, tolerating thousands separators and blanks.
func atoiLoose(s string) int {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	n, _ := strconv.Atoi(s)
	return n
}

// decodeKorean returns text decoded as UTF-8 when valid, otherwise from EUC-KR
// (a superset of which, CP949, is what NEC 개표결과 CSVs use).
func decodeKorean(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if utf8.Valid(raw) {
		return string(raw)
	}
	if dec, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), korean.EUCKR.NewDecoder())); err == nil {
		return string(dec)
	}
	return string(raw)
}
