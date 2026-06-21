package nec

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Dataset is one NEC file dataset on data.go.kr. PublicDataPk is the stable
// identifier used by `nec pull`.
type Dataset struct {
	PublicDataPk string   `json:"publicDataPk"`
	Title        string   `json:"title"`
	Description  string   `json:"description,omitempty"`
	Formats      []string `json:"formats,omitempty"`
}

// SearchOptions filters a dataset search.
type SearchOptions struct {
	Keyword string // free-text query (searched within NEC datasets)
	Org     string // publisher name; defaults to 중앙선거관리위원회
	Page    int    // 1-based page
}

var (
	rePk         = regexp.MustCompile(`/data/(\d+)/fileData\.do`)
	knownFormats = map[string]bool{"CSV": true, "XLSX": true, "XLS": true, "JSON": true, "XML": true, "HWP": true, "PDF": true, "ZIP": true}
)

// Datasets searches data.go.kr file datasets published by the NEC. Results are
// scoped to the commission via the org filter, so a blank keyword lists the
// NEC's datasets.
func (c *Client) Datasets(ctx context.Context, opts SearchOptions) ([]Dataset, error) {
	org := opts.Org
	if org == "" {
		org = DefaultOrg
	}
	page := opts.Page
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	q.Set("dType", "FILE")
	q.Set("org", org)
	q.Set("keyword", opts.Keyword)
	q.Set("currentPage", itoa(page))
	q.Set("perPage", "10")

	doc, err := c.getDoc(ctx, "/tcs/dss/selectDataSetList.do", q)
	if err != nil {
		return nil, err
	}

	var out []Dataset
	seen := map[string]bool{}
	doc.Find("dl").Each(func(_ int, dl *goquery.Selection) {
		href, _ := dl.Find(`a[href*="/data/"]`).First().Attr("href")
		m := rePk.FindStringSubmatch(href)
		if m == nil || seen[m[1]] {
			return
		}
		seen[m[1]] = true
		title, formats := parseDt(dl)
		out = append(out, Dataset{
			PublicDataPk: m[1],
			Title:        title,
			Description:  cleanText(dl.Find("dd").First().Text()),
			Formats:      formats,
		})
	})
	return out, nil
}

// parseDt splits a dataset heading into its distribution formats and clean
// title. The <dt> text leads with format labels ("CSV", "JSON + XML", "XLSX"),
// followed by the dataset name, with a trailing screen-reader "미리보기".
func parseDt(dl *goquery.Selection) (title string, formats []string) {
	text := cleanText(dl.Find("dt").First().Text())
	toks := strings.Fields(text)

	seen := map[string]bool{}
	i := 0
	for i < len(toks) {
		up := strings.ToUpper(toks[i])
		if knownFormats[up] {
			if !seen[up] {
				seen[up] = true
				formats = append(formats, up)
			}
			i++
			continue
		}
		if toks[i] == "+" { // joins multi-format labels like "JSON + XML"
			i++
			continue
		}
		break
	}
	title = strings.TrimSpace(strings.TrimSuffix(strings.Join(toks[i:], " "), "미리보기"))
	if title == "" { // fall back to the description's bracketed election name
		title = bracketName(cleanText(dl.Find("dd").First().Text()))
	}
	return title, formats
}

// bracketName returns the text inside the leading "[...]" of a description.
func bracketName(s string) string {
	if i := strings.IndexByte(s, '['); i >= 0 {
		if j := strings.IndexByte(s[i:], ']'); j > 0 {
			return strings.TrimSpace(s[i+1 : i+j])
		}
	}
	return s
}
