package nesdc

import (
	"context"
	"net/url"
	"regexp"

	"github.com/PuerkitoBio/goquery"
)

// Agency is one row of the polling-agency registry (여론조사기관 등록/취소 현황).
type Agency struct {
	InsttNum  string            `json:"insttNum"`
	Cancelled bool              `json:"cancelled"`
	Values    map[string]string `json:"values"`
}

// AgencyResult is a parsed page of the agency registry.
type AgencyResult struct {
	Cancelled bool     `json:"cancelled"`
	Page      int      `json:"page"`
	Columns   []string `json:"columns"`
	Items     []Agency `json:"items"`
}

var reInsttNum = regexp.MustCompile(`insttNum=(\d+)`)

// Agencies fetches one page of the agency registry. When cancelled is true the
// cancellation list (취소현황) is returned instead of the active registry.
func (c *Client) Agencies(ctx context.Context, cancelled bool, opts ListOptions) (*AgencyResult, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	q := url.Values{}
	q.Set("pageIndex", itoa(page))
	if opts.Keyword != "" {
		cnd := opts.SearchCnd
		if cnd == "" {
			cnd = "0"
		}
		q.Set("searchCnd", cnd)
		q.Set("searchWrd", opts.Keyword)
	}
	if opts.From != "" {
		q.Set("sdate", opts.From)
	}
	if opts.To != "" {
		q.Set("edate", opts.To)
	}

	path := "/content/onvy/list.do"
	if cancelled {
		path = "/content/onvy/cancel.do"
	}
	doc, err := c.getDoc(ctx, path, q)
	if err != nil {
		return nil, err
	}

	res := &AgencyResult{Cancelled: cancelled, Page: page, Columns: parseHeader(doc)}
	doc.Find(".row.tr").Each(func(_ int, row *goquery.Selection) {
		num := insttNumOf(row)
		if num == "" {
			return
		}
		cells := cellTexts(row)
		item := Agency{InsttNum: num, Cancelled: cancelled, Values: map[string]string{}}
		for i, cell := range cells {
			item.Values[columnKey(res.Columns, i)] = cell
		}
		res.Items = append(res.Items, item)
	})
	return res, nil
}

// insttNumOf extracts the institution number from a row's anchor.
func insttNumOf(row *goquery.Selection) string {
	if href, ok := row.Attr("href"); ok {
		if m := reInsttNum.FindStringSubmatch(href); m != nil {
			return m[1]
		}
	}
	var num string
	row.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		if m := reInsttNum.FindStringSubmatch(href); m != nil {
			num = m[1]
			return false
		}
		return true
	})
	return num
}
