package nec

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// TurnoutAnalysisRecord is one normalized (지역 × 성별 × 연령대) turnout cell from a NEC
// "투표율 분석" dataset. Rate is the source-reported value (원자료), not derived.
type TurnoutAnalysisRecord struct {
	Election    string  `json:"election"`
	Category    string  `json:"category"`
	RegionLevel string  `json:"regionLevel"`
	Sido        string  `json:"sido"`
	Region      string  `json:"region"`
	Gender      string  `json:"gender"`
	AgeGroup    string  `json:"ageGroup"`
	Electorate  int     `json:"electorate"`
	Voters      int     `json:"voters"`
	Rate        float64 `json:"rate"`
}

// ParseTurnoutAnalysis unzips a NEC 투표율 분석 dataset and parses every sheet that
// matches the 성별·연령대별 투표율 cross-tab anchor into long-format records. Files
// and sheets that don't match are skipped with a stderr warning (like ParseResultsXLSX).
// Election is left empty; the caller fills it from the dataset name.
func ParseTurnoutAnalysis(zipRaw []byte) ([]TurnoutAnalysisRecord, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipRaw), int64(len(zipRaw)))
	if err != nil {
		return nil, fmt.Errorf("open zip (nec pull 로 원본 확인): %w", err)
	}
	var out []TurnoutAnalysisRecord
	for _, zf := range zr.File {
		if !strings.HasSuffix(strings.ToLower(zf.Name), ".xlsx") {
			continue
		}
		if !strings.Contains(zf.Name, "성별") { // target filename hint
			continue
		}
		raw, err := readZipEntry(zf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %q: %v\n", zf.Name, err)
			continue
		}
		recs := parseTurnoutXLSX(zf.Name, raw)
		out = append(out, recs...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("성별·연령대별 투표율 시트를 찾지 못했습니다 (사전/재외/PDF 데이터셋일 수 있음 — nec pull 로 원본 확인)")
	}
	return out, nil
}

func readZipEntry(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// parseTurnoutXLSX parses one xlsx (multi-sheet by 시도) into records. Non-matching
// sheets are skipped with a warning; a malformed workbook yields no records.
func parseTurnoutXLSX(fname string, raw []byte) []TurnoutAnalysisRecord {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "skip %q: open xlsx: %v\n", fname, err)
		return nil
	}
	defer f.Close()

	var out []TurnoutAnalysisRecord
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) < 5 {
			continue
		}
		recs := parseTurnoutSheet(sheet, rows)
		out = append(out, recs...)
	}
	return out
}

// cellAt returns row[i] trimmed, or "" when out of range.
func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func norm(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), " ", "") }

// parseTurnoutSheet parses a single 시도 sheet. Returns nil when the sheet lacks
// the 성별·연령대별 anchor (구분 header + 선거인수/투표자수/투표율 rows).
func parseTurnoutSheet(sheet string, rows [][]string) []TurnoutAnalysisRecord {
	regionLevel := ""
	category := ""
	headerRow := -1
	ageCols := []int{}      // column indices of age buckets
	ageLabels := []string{} // parallel labels

	for i, row := range rows {
		c0 := cellAt(row, 0)
		if strings.Contains(c0, "구시군별") {
			regionLevel = "구시군"
		} else if strings.Contains(c0, "선거구별") {
			regionLevel = "선거구"
		}
		if strings.HasPrefix(c0, "[") {
			if end := strings.Index(c0, "]"); end > 1 {
				category = c0[1:end]
			}
		}
		if norm(c0) == "구분" {
			headerRow = i
			for j := 3; j < len(row); j++ {
				lbl := cellAt(row, j)
				if lbl == "" {
					continue
				}
				ageCols = append(ageCols, j)
				ageLabels = append(ageLabels, lbl)
			}
			break
		}
	}
	if headerRow < 0 || len(ageCols) == 0 {
		return nil // not a target sheet
	}

	var out []TurnoutAnalysisRecord
	curRegion := ""
	// walk data rows in groups: col0 region (sticky), col1 gender, col2 metric.
	// A (region,gender) block is 3 consecutive metric rows: 선거인수/투표자수/투표율.
	i := headerRow + 1
	for i < len(rows) {
		row := rows[i]
		if r := cellAt(row, 0); r != "" {
			curRegion = r
		}
		gender := cellAt(row, 1)
		metric := norm(cellAt(row, 2))
		if curRegion == "" || gender == "" || metric != "선거인수" {
			i++
			continue
		}
		// Expect this row = 선거인수, next = 투표자수, next+1 = 투표율.
		elecRow := rows[i]
		var voteRow, rateRow []string
		if i+1 < len(rows) {
			voteRow = rows[i+1]
		}
		if i+2 < len(rows) {
			rateRow = rows[i+2]
		}
		if norm(cellAt(voteRow, 2)) != "투표자수" || norm(cellAt(rateRow, 2)) != "투표율" {
			i++
			continue
		}
		for k, col := range ageCols {
			out = append(out, TurnoutAnalysisRecord{
				Category:    category,
				RegionLevel: regionLevel,
				Sido:        sheet,
				Region:      curRegion,
				Gender:      gender,
				AgeGroup:    ageLabels[k],
				Electorate:  atoiLoose(cellAt(elecRow, col)),
				Voters:      atoiLoose(cellAt(voteRow, col)),
				Rate:        parseRate(cellAt(rateRow, col)),
			})
		}
		i += 3
	}
	return out
}

// parseRate parses a percentage like "69.4"; blank or "-" yields 0.
func parseRate(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
