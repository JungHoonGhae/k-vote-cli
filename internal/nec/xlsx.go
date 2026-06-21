package nec

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

// candHeader is a per-column candidate identity, populated by candidate-definition rows.
type candHeader struct {
	Party string
	Name  string
}

// isGyoEducSheet returns true when the sheet name indicates a 교육감 or 교육의원
// race, which is non-partisan by law: single-part candidate cells carry a
// person NAME, not a party name.
func isGyoEducSheet(sheetName string) bool {
	return strings.Contains(sheetName, "교육감") || strings.Contains(sheetName, "교육의원")
}

// ParseResultsXLSX parses a NEC 개표결과 XLSX file (multi-sheet, wide format) into
// a slice of ElectionResult in long format. Each sheet becomes one Race. Sheets that
// lack the required anchor columns (선거인수 / 후보자별 득표수) are skipped with a
// warning to stderr. An error is returned only if no sheet produced any record.
func ParseResultsXLSX(raw []byte) ([]ElectionResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var out []ElectionResult

	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip sheet %q: GetRows error: %v\n", name, err)
			continue
		}
		if len(rows) < 2 {
			fmt.Fprintf(os.Stderr, "skip sheet %q: missing anchor\n", name)
			continue
		}

		// row0 is the label row; locate anchor columns.
		row0 := rows[0]

		// Helper to get cell text from row0 with space removal for comparison.
		labelAt := func(i int) string {
			if i >= len(row0) {
				return ""
			}
			return strings.TrimSpace(row0[i])
		}
		labelNorm := func(i int) string {
			return strings.ReplaceAll(labelAt(i), " ", "")
		}

		// Find electIdx (선거인수).
		electIdx := -1
		for i := range row0 {
			if labelNorm(i) == "선거인수" {
				electIdx = i
				break
			}
		}
		if electIdx < 0 {
			fmt.Fprintf(os.Stderr, "skip sheet %q: missing anchor\n", name)
			continue
		}

		// Find votesIdx (투표수) after electIdx.
		votesIdx := -1
		for i := electIdx + 1; i < len(row0); i++ {
			if labelNorm(i) == "투표수" {
				votesIdx = i
				break
			}
		}

		// Find candStart (후보자별 득표수).
		candStart := -1
		for i := range row0 {
			if labelNorm(i) == "후보자별득표수" {
				candStart = i
				break
			}
		}
		if candStart < 0 {
			fmt.Fprintf(os.Stderr, "skip sheet %q: missing anchor\n", name)
			continue
		}

		// Find gyeIdx (계), invalidIdx (무효투표수), abstIdx (기권수).
		gyeIdx := -1
		invalidIdx := -1
		abstIdx := -1
		for i := candStart + 1; i < len(row0); i++ {
			n := labelNorm(i)
			switch n {
			case "계":
				if gyeIdx < 0 {
					gyeIdx = i
				}
			case "무효투표수":
				if invalidIdx < 0 {
					invalidIdx = i
				}
			case "기권수":
				if abstIdx < 0 {
					abstIdx = i
				}
			}
		}

		// Candidate block is [candStart, gyeIdx). If gyeIdx not found, fall back to
		// len(row0) so every remaining column is treated as a candidate slot.
		candEnd := len(row0)
		if gyeIdx >= 0 {
			candEnd = gyeIdx
		}

		// Dimension columns are [0, electIdx). Compute their labels once.
		dimLabels := make([]string, electIdx)
		for i := 0; i < electIdx; i++ {
			dimLabels[i] = labelAt(i)
		}

		// Find index of 구분 within dimension columns (used for vote-type derivation).
		// Use the same space-normalisation as the rest of the label comparisons (labelNorm).
		gubunDimIdx := -1 // 구분
		for i := 0; i < electIdx; i++ {
			if labelNorm(i) == "구분" {
				gubunDimIdx = i
				break
			}
		}

		// candHeaders tracks the running candidate header (party, name) per column.
		// Initialized to nil; nil entry means that column slot has no candidate.
		candHeaders := make([]*candHeader, candEnd-candStart)

		cellAt := func(row []string, i int) string {
			if i < 0 || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}

		isAllEmpty := func(row []string) bool {
			for _, c := range row {
				if strings.TrimSpace(c) != "" {
					return false
				}
			}
			return true
		}

		// gyoEduc records whether this sheet uses person-name semantics for
		// single-part candidate cells (교육감/교육의원 are non-partisan by law).
		gyoEduc := isGyoEducSheet(name)

		// Process data rows starting from index 2 (skip row0=labels, row1=merged remnants).
		for ri := 2; ri < len(rows); ri++ {
			row := rows[ri]

			// Skip wholly-empty rows.
			if isAllEmpty(row) {
				continue
			}

			// Detect a candidate-definition row using the 선거인수 anchor:
			// header rows carry no numbers — real data rows always have 선거인수.
			// Additionally require at least one candidate cell to be non-empty.
			// The dimension-empty check is kept as corroboration but the
			// electorate-empty signal is the primary discriminator, ensuring
			// correct behaviour even when 읍면동명/구분 columns are absent.
			someCandNonEmpty := false
			for j := 0; j < len(candHeaders); j++ {
				if cellAt(row, candStart+j) != "" {
					someCandNonEmpty = true
					break
				}
			}

			isCandDef := someCandNonEmpty && cellAt(row, electIdx) == ""

			if isCandDef {
				// Update candidate header for every candidate column.
				for j := 0; j < len(candHeaders); j++ {
					colIdx := candStart + j
					v := cellAt(row, colIdx)
					if v == "" {
						candHeaders[j] = nil
						continue
					}
					parts := strings.Split(v, "\n")
					if len(parts) >= 2 {
						candHeaders[j] = &candHeader{
							Party: strings.TrimSpace(parts[0]),
							Name:  strings.TrimSpace(parts[1]),
						}
					} else {
						// Single-part candidate cell: sheet semantics determine mapping.
						// 교육감/교육의원 races are non-partisan — single part is a NAME.
						// All other sheets (비례대표 etc.) — single part is a PARTY.
						s := strings.TrimSpace(parts[0])
						if gyoEduc {
							candHeaders[j] = &candHeader{Party: "", Name: s}
						} else {
							candHeaders[j] = &candHeader{Party: s, Name: ""}
						}
					}
				}
				continue // do not emit as a record
			}

			// Regular data row: build ElectionResult.
			dims := make([]Dimension, electIdx)
			for i := 0; i < electIdx; i++ {
				dims[i] = Dimension{
					Label: dimLabels[i],
					Value: cellAt(row, i),
				}
			}

			// Derive vote type and aggregate flag from 구분 dimension.
			gubun := ""
			if gubunDimIdx >= 0 {
				gubun = cellAt(row, gubunDimIdx)
			}
			voteType, aggregate := deriveVoteType(gubun)

			// Collect candidates.
			var candidates []CandidateVote
			for j := 0; j < len(candHeaders); j++ {
				if candHeaders[j] == nil {
					continue
				}
				colIdx := candStart + j
				v := cellAt(row, colIdx)
				if v == "" {
					continue
				}
				candidates = append(candidates, CandidateVote{
					Party: candHeaders[j].Party,
					Name:  candHeaders[j].Name,
					Votes: atoiLoose(v),
				})
			}

			rec := ElectionResult{
				Race:       name,
				Dimensions: dims,
				VoteType:   voteType,
				Aggregate:  aggregate,
				Electorate: atoiLoose(cellAt(row, electIdx)),
				Candidates: candidates,
			}
			if votesIdx >= 0 {
				rec.Votes = atoiLoose(cellAt(row, votesIdx))
			}
			if invalidIdx >= 0 {
				rec.Invalid = atoiLoose(cellAt(row, invalidIdx))
			}
			if abstIdx >= 0 {
				rec.Abstention = atoiLoose(cellAt(row, abstIdx))
			}

			out = append(out, rec)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no records parsed from any sheet")
	}
	return out, nil
}
