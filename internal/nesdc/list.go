package nesdc

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ListOptions filters a board listing. Zero values mean "no filter".
type ListOptions struct {
	Page       int    // pageIndex (1-based); 0 → 1
	Keyword    string // searchWrd
	SearchCnd  string // searchCnd (search field selector); default "0"
	SearchTime string // searchTime: date field that From/To apply to (1 등록일 / 2 최초공표일 / 3 조사일시)
	From       string // sdate, format YYYY-MM-DD
	To         string // edate, format YYYY-MM-DD
	PollGubun  string // pollGubuncd (results board only)
}

// ListItem is one row of a board listing. NttID is the stable post identifier;
// Values maps the board's column headers to this row's cell text.
type ListItem struct {
	NttID  string            `json:"nttId"`
	Board  string            `json:"board"`
	Values map[string]string `json:"values"`
}

// ListResult is a parsed page of a board listing.
type ListResult struct {
	Board   string     `json:"board"`
	Page    int        `json:"page"`
	Columns []string   `json:"columns"`
	Items   []ListItem `json:"items"`
}

var reNttID = regexp.MustCompile(`nttId=(\d+)`)

// List fetches and parses one page of a board listing.
func (c *Client) List(ctx context.Context, b Board, opts ListOptions) (*ListResult, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	q.Set("menuNo", b.MenuNo)
	q.Set("pageIndex", itoa(page))
	if opts.Keyword != "" {
		cnd := opts.SearchCnd
		if cnd == "" {
			cnd = "0"
		}
		q.Set("searchCnd", cnd)
		q.Set("searchWrd", opts.Keyword)
	}
	// The portal ignores sdate/edate unless searchTime names which date field to
	// filter on, so default to 등록일 (1) whenever a range is given.
	if opts.From != "" || opts.To != "" {
		st := opts.SearchTime
		if st == "" {
			st = "1"
		}
		q.Set("searchTime", st)
		if opts.From != "" {
			q.Set("sdate", opts.From)
		}
		if opts.To != "" {
			q.Set("edate", opts.To)
		}
	}
	if opts.PollGubun != "" {
		q.Set("pollGubuncd", opts.PollGubun)
	}

	doc, err := c.getDoc(ctx, "/bbs/"+b.BbsID+"/list.do", q)
	if err != nil {
		return nil, err
	}

	columns := parseHeader(doc)
	res := &ListResult{Board: b.Name, Page: page, Columns: columns}

	doc.Find(".row.tr").Each(func(_ int, row *goquery.Selection) {
		nttID := nttIDOf(row)
		if nttID == "" {
			return
		}
		cells := cellTexts(row)
		item := ListItem{NttID: nttID, Board: b.Name, Values: map[string]string{}}
		for i, cell := range cells {
			key := columnKey(columns, i)
			item.Values[key] = cell
		}
		res.Items = append(res.Items, item)
	})
	return res, nil
}

// parseHeader reads the ".row.th" labels that name each column.
func parseHeader(doc *goquery.Document) []string {
	var cols []string
	doc.Find(".row.th").First().Find(".col").Each(func(_ int, s *goquery.Selection) {
		cols = append(cols, cleanText(s.Text()))
	})
	return cols
}

// cellTexts returns the text of every ".col" in a row, preserving empties so
// the slice stays aligned with the header.
func cellTexts(row *goquery.Selection) []string {
	var out []string
	row.Find(".col").Each(func(_ int, s *goquery.Selection) {
		out = append(out, cleanText(s.Text()))
	})
	return out
}

// nttIDOf extracts the nttId from a row's own href or a nested anchor.
func nttIDOf(row *goquery.Selection) string {
	if href, ok := row.Attr("href"); ok {
		if m := reNttID.FindStringSubmatch(href); m != nil {
			return m[1]
		}
	}
	var id string
	row.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		if m := reNttID.FindStringSubmatch(href); m != nil {
			id = m[1]
			return false
		}
		return true
	})
	return id
}

// columnKey returns the header label for column i, or a positional fallback.
func columnKey(columns []string, i int) string {
	if i < len(columns) && columns[i] != "" {
		return columns[i]
	}
	return "col" + itoa(i+1)
}

// cleanText collapses whitespace (including the non-breaking spaces the portal
// litters its tables with) into single spaces.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
