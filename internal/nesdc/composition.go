package nesdc

import (
	"strconv"
	"strings"
)

// CompositionCell is one demographic category's sample counts.
type CompositionCell struct {
	Category  string `json:"category"`
	Completed int    `json:"completed"`
	Weighted  int    `json:"weighted"`
}

// Crosstab is one demographic dimension's breakdown of the sample.
type Crosstab struct {
	Dimension string            `json:"dimension"`
	Cells     []CompositionCell `json:"cells"`
}

// SampleComposition is the structured 표본 구성: who was sampled (completed vs
// weighted counts) by 성별/연령대별/지역별, plus the weighting method and margin
// of error. All raw counts are preserved; nothing is interpreted.
type SampleComposition struct {
	Total       *CompositionCell `json:"total,omitempty"`
	Crosstabs   []Crosstab       `json:"crosstabs"`
	Weighting   string           `json:"weighting,omitempty"`
	MarginError string           `json:"marginError,omitempty"`
}

// SampleCompositionOf derives the sample-composition crosstab from a detail
// page's already-parsed Fields. Returns nil when the block is absent.
func SampleCompositionOf(d *Detail) *SampleComposition {
	start := compositionHeaderIndex(d.Fields)
	if start < 0 {
		return nil
	}
	sc := &SampleComposition{}
	var cur *Crosstab
	for _, f := range d.Fields[start+1:] {
		completed, weighted, ok := twoInts(f.Values)
		if !ok {
			break // first non-(int,int) row ends the block
		}
		switch len(f.Labels) {
		case 0:
			continue
		case 1:
			if f.Labels[0] == "전체" {
				sc.Total = &CompositionCell{Category: "전체", Completed: completed, Weighted: weighted}
				continue
			}
			if cur != nil {
				cur.Cells = append(cur.Cells, CompositionCell{Category: f.Labels[0], Completed: completed, Weighted: weighted})
			}
		default: // >=2 labels: [dimension, category]
			sc.Crosstabs = append(sc.Crosstabs, Crosstab{Dimension: f.Labels[0]})
			cur = &sc.Crosstabs[len(sc.Crosstabs)-1]
			cur.Cells = append(cur.Cells, CompositionCell{Category: f.Labels[1], Completed: completed, Weighted: weighted})
		}
	}
	sc.Weighting = fieldValueWhereLabel(d.Fields, "산출방법")
	sc.MarginError = fieldValueWhereLabel(d.Fields, "표본오차")
	if sc.Total == nil && len(sc.Crosstabs) == 0 {
		return nil
	}
	return sc
}

func compositionHeaderIndex(fields []Field) int {
	for i, f := range fields {
		joined := strings.ReplaceAll(strings.Join(f.Labels, ""), " ", "")
		if strings.Contains(joined, "구분") && strings.Contains(joined, "조사완료사례수") {
			return i
		}
	}
	return -1
}

// twoInts parses exactly two integer values (comma-stripped). ok=false when the
// slice is not two parseable integers.
func twoInts(vals []string) (a, b int, ok bool) {
	if len(vals) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(vals[0]), ",", ""))
	b, err2 := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(vals[1]), ",", ""))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

// fieldValueWhereLabel returns the first value of the first field whose labels
// contain the given label, or "".
func fieldValueWhereLabel(fields []Field, label string) string {
	for _, f := range fields {
		for _, l := range f.Labels {
			if l == label && len(f.Values) > 0 {
				return f.Values[0]
			}
		}
	}
	return ""
}
