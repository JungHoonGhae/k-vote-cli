package nec

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// LatestRef is the newest dataset of an election type found in one source.
// Era is the 회차/대수 parsed from the title (제8회→8, 제21대→21).
type LatestRef struct {
	Source string `json:"source"` // datagokr | openportal
	Key    string `json:"key"`    // publicDataPk (datagokr) 또는 dataId (openportal)
	Era    int    `json:"era"`
	Title  string `json:"title"`
}

var reEra = regexp.MustCompile(`제(\d+)\s*[회대]`)

// LatestDataset resolves the highest-era dataset matching keyword in each source
// (data.go.kr and the open portal). keyword should name one election type, e.g.
// "지방선거 개표결과" — eras are only comparable within a single type (지방선거 회차
// vs 대선 대수 are different series). Sources that error or have no match are
// simply omitted; the result is empty only when neither yields a dated dataset.
func (c *Client) LatestDataset(ctx context.Context, keyword string) []LatestRef {
	var out []LatestRef

	tokens := strings.Fields(keyword)

	if ds, err := c.Datasets(ctx, SearchOptions{Keyword: keyword}); err == nil {
		pairs := make([][2]string, len(ds))
		for i, d := range ds {
			pairs[i] = [2]string{d.PublicDataPk, d.Title}
		}
		if r, ok := latestOf(string(SourceDataGoKr), pairs, tokens); ok {
			out = append(out, r)
		}
	}

	if ds, err := c.OpenPortalDatasets(ctx, keyword); err == nil {
		pairs := make([][2]string, len(ds))
		for i, d := range ds {
			pairs[i] = [2]string{d.DataID, d.Title}
		}
		if r, ok := latestOf(string(SourceOpenPortal), pairs, tokens); ok {
			out = append(out, r)
		}
	}

	return out
}

// latestOf returns the highest-era (key,title) pair among titles containing
// every keyword token. The token filter prevents two failure modes seen with
// the portals' loose search: a different election type leaking in (제17대 대선
// for a "지방선거" query — and 회 vs 대 are not comparable), and a sibling dataset
// of the same era being picked (유권자 의식조사 instead of 개표결과).
func latestOf(source string, items [][2]string, tokens []string) (LatestRef, bool) {
	var best LatestRef
	found := false
	for _, it := range items {
		title := it[1]
		if !containsAll(title, tokens) {
			continue
		}
		m := reEra.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		era, _ := strconv.Atoi(m[1])
		if !found || era > best.Era {
			best = LatestRef{Source: source, Key: it[0], Era: era, Title: title}
			found = true
		}
	}
	return best, found
}

func containsAll(s string, tokens []string) bool {
	for _, t := range tokens {
		if !strings.Contains(s, t) {
			return false
		}
	}
	return true
}
