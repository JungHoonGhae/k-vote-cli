package nesdc

import (
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// PollRecord is one normalized row of the cumulative "주요 데이터" workbook: a
// single registered poll with its metadata and per-party support figures. The
// fixed metadata columns are mapped to named fields; the variable party columns
// (which change over time as parties appear and merge) are kept as an ordered
// map so no figure is lost.
type PollRecord struct {
	Period       string            `json:"period"` // source sheet (date range)
	RegNo        string            `json:"regNo,omitempty"`
	Agency       string            `json:"agency,omitempty"`
	Client       string            `json:"client,omitempty"`
	SurveyDate   string            `json:"surveyDate,omitempty"`
	Method       string            `json:"method,omitempty"`
	Frame        string            `json:"frame,omitempty"`
	SampleSize   string            `json:"sampleSize,omitempty"`
	ContactRate  string            `json:"contactRate,omitempty"`
	ResponseRate string            `json:"responseRate,omitempty"`
	MarginError  string            `json:"marginError,omitempty"`
	PartySupport map[string]string `json:"partySupport,omitempty"`
}

// metaCols maps PollRecord fields to a substring of the workbook's first-row
// label. The source labels carry suffixes ("표본수(명)", "응답률(%)",
// "95%신뢰수준\n표본오차(%p)"), so matching is by substring, not equality. Order
// matters: "조사일자" must be tried before "조사방법"/"조사기관" would never
// collide, but keeping declaration order explicit guards against future labels.
var metaCols = []struct {
	key string
	set func(*PollRecord, string)
}{
	{"등록번호", func(r *PollRecord, v string) { r.RegNo = v }},
	{"조사기관", func(r *PollRecord, v string) { r.Agency = v }},
	{"의뢰자", func(r *PollRecord, v string) { r.Client = v }},
	{"조사일자", func(r *PollRecord, v string) { r.SurveyDate = v }},
	{"조사방법", func(r *PollRecord, v string) { r.Method = v }},
	{"표본추출틀", func(r *PollRecord, v string) { r.Frame = v }},
	{"표본수", func(r *PollRecord, v string) { r.SampleSize = v }},
	{"접촉률", func(r *PollRecord, v string) { r.ContactRate = v }},
	{"응답률", func(r *PollRecord, v string) { r.ResponseRate = v }},
	{"표본오차", func(r *PollRecord, v string) { r.MarginError = v }},
}

const partyHeader = "정당지지율"

// LatestBulkXlsx finds the cumulative master workbook attached to the most
// recent post of the data board and returns its attachment. The data board
// re-attaches the same growing .xlsx ("전국단위 …") to every weekly post, so the
// newest post always carries the freshest copy.
func (c *Client) LatestBulkXlsx(ctx context.Context, b Board) (Attachment, error) {
	list, err := c.List(ctx, b, ListOptions{Page: 1})
	if err != nil {
		return Attachment{}, err
	}
	if len(list.Items) == 0 {
		return Attachment{}, fmt.Errorf("data board has no posts")
	}
	for _, it := range list.Items {
		d, err := c.Detail(ctx, b, it.NttID)
		if err != nil {
			return Attachment{}, err
		}
		for _, a := range d.Attachments {
			if strings.HasSuffix(strings.ToLower(a.Name), ".xlsx") {
				return a, nil
			}
		}
	}
	return Attachment{}, fmt.Errorf("no .xlsx attachment found on recent data-board posts")
}

// ParseBulkXlsx reads the cumulative workbook at path into normalized records,
// one per poll, across every sheet. Each sheet uses a two-row header: fixed
// metadata labels on the first row, and — under the merged 정당지지율 cell —
// individual party names on the second row.
func ParseBulkXlsx(path string) ([]PollRecord, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []PollRecord
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", sheet, err)
		}
		if len(rows) < 3 {
			continue
		}
		out = append(out, parseSheet(sheet, rows)...)
	}
	return out, nil
}

// parseSheet turns the data rows of one sheet into PollRecords using its
// two-row header (rows[0] = metadata labels, rows[1] = party names).
func parseSheet(sheet string, rows [][]string) []PollRecord {
	head, parties := rows[0], rows[1]

	// partyStart is the first column belonging to the party-support block.
	partyStart := len(head)
	for i, h := range head {
		if strings.Contains(collapseWS(h), partyHeader) {
			partyStart = i
			break
		}
	}

	var out []PollRecord
	for _, row := range rows[2:] {
		if regNo := cell(row, 0); regNo == "" || regNo == "·" {
			continue // spacer / empty row
		}
		rec := PollRecord{Period: sheet}
		for c := 0; c < partyStart; c++ {
			label := collapseWS(cell(head, c))
			for _, m := range metaCols {
				if strings.Contains(label, m.key) {
					m.set(&rec, cell(row, c))
					break
				}
			}
		}
		for c := partyStart; c < len(parties); c++ {
			name := collapseWS(cell(parties, c))
			val := cell(row, c)
			if name == "" || val == "" || val == "·" {
				continue
			}
			if rec.PartySupport == nil {
				rec.PartySupport = map[string]string{}
			}
			rec.PartySupport[name] = val
		}
		out = append(out, rec)
	}
	return out
}

// cell returns row[i] trimmed, or "" if out of range.
func cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// collapseWS removes interior newlines/spaces so multi-line header labels like
// "95%신뢰수준\n표본오차(%p)" and "표본수(명)" match their short keys.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), "")
}
