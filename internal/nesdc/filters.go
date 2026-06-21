package nesdc

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// Election is one option of the results board's 선거구분 (pollGubuncd) filter.
type Election struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Elections scrapes the live 선거구분 dropdown from the results board so callers
// can discover valid --gubun codes without an API key. The empty placeholder
// option (":: 선거구분 ::") is omitted.
func (c *Client) Elections(ctx context.Context) ([]Election, error) {
	b, err := BoardByName("results")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("menuNo", b.MenuNo)
	doc, err := c.getDoc(ctx, "/bbs/"+b.BbsID+"/list.do", q)
	if err != nil {
		return nil, err
	}
	var out []Election
	doc.Find(`select[name="pollGubuncd"] option`).Each(func(_ int, s *goquery.Selection) {
		code, _ := s.Attr("value")
		name := cleanText(s.Text())
		if code == "" || strings.Contains(name, "선거구분") {
			return
		}
		out = append(out, Election{Code: code, Name: name})
	})
	return out, nil
}
