package nesdc

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Field is one labelled row of a detail page's metadata table. A row may carry
// several stacked labels (e.g. ["성별", "남"]) and several values, so both are
// preserved as slices to stay lossless.
type Field struct {
	Labels []string `json:"labels"`
	Values []string `json:"values"`
}

// Attachment is a downloadable file referenced by a detail page.
type Attachment struct {
	Name       string `json:"name"`
	AtchFileID string `json:"atchFileId"`
	FileSn     string `json:"fileSn"`
	BbsID      string `json:"bbsId"`
	BbsKey     string `json:"bbsKey"`
}

// Detail is a parsed view.do page. Fields is the complete, ordered metadata
// table (lossless); Summary surfaces a curated set of headline scalars for
// quick consumption.
type Detail struct {
	NttID       string            `json:"nttId"`
	Board       string            `json:"board"`
	Title       string            `json:"title,omitempty"`
	Summary     map[string]string `json:"summary,omitempty"`
	Fields      []Field           `json:"fields"`
	Attachments []Attachment      `json:"attachments"`
}

// view('atchFileId', 'fileSn', 'bbsId', 'bbsKey')
var reViewCall = regexp.MustCompile(`view\(\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*,\s*'([^']*)'\s*\)`)

// summaryLabels are single-label rows worth surfacing in Summary.
var summaryLabels = map[string]bool{
	"등록 글번호":        true,
	"조사기관명":         true,
	"조사의뢰자":         true,
	"공동조사기관명":       true,
	"조사일시":          true,
	"조사대상":          true,
	"조사방법1":         true,
	"표본오차":          true,
	"전체 응답률":        true,
	"전체 접촉률":        true,
	"적용방법":          true,
	"공표·보도 매체명":     true,
	"최초 공표·보도 지정일시": true,
}

// Detail fetches and parses a view.do page for the given board and post id.
func (c *Client) Detail(ctx context.Context, b Board, nttID string) (*Detail, error) {
	q := url.Values{}
	q.Set("nttId", nttID)
	q.Set("menuNo", b.MenuNo)

	doc, err := c.getDoc(ctx, "/bbs/"+b.BbsID+"/view.do", q)
	if err != nil {
		return nil, err
	}

	d := &Detail{
		NttID:   nttID,
		Board:   b.Name,
		Summary: map[string]string{},
	}
	d.Fields = parseFields(doc)
	d.Title = parseTitle(doc, d.Fields)
	for _, f := range d.Fields {
		if len(f.Labels) == 1 && summaryLabels[f.Labels[0]] && len(f.Values) > 0 {
			d.Summary[f.Labels[0]] = strings.Join(f.Values, " ")
		}
	}
	if len(d.Summary) == 0 {
		d.Summary = nil
	}
	d.Attachments = parseAttachments(doc)
	return d, nil
}

// titleFieldLabels are clean single-label fields (in priority order) used as a
// post title when the markup has no dedicated heading element. Multi-label rows
// (e.g. "여론조사 명칭" paired with "선거구분") are skipped to avoid grabbing a
// neighbouring cell's value.
var titleFieldLabels = []string{"선거명", "제목"}

// parseTitle pulls the post heading from the markup, falling back to a
// meaningful single-label field value. It deliberately avoids the generic
// ".tit" class, which the portal also uses for empty layout markers.
func parseTitle(doc *goquery.Document, fields []Field) string {
	for _, sel := range []string{".bbsV_tit", ".view_tit", ".board_view_top .tit"} {
		if t := cleanText(doc.Find(sel).First().Text()); t != "" {
			return t
		}
	}
	for _, want := range titleFieldLabels {
		for _, f := range fields {
			if len(f.Labels) == 1 && f.Labels[0] == want && len(f.Values) > 0 {
				return strings.Join(f.Values, " ")
			}
		}
	}
	return ""
}

// parseFields walks every table row, collecting th labels and td values.
func parseFields(doc *goquery.Document) []Field {
	var fields []Field
	doc.Find("table tr").Each(func(_ int, tr *goquery.Selection) {
		var labels, values []string
		tr.Find("th").Each(func(_ int, th *goquery.Selection) {
			if t := cleanText(th.Text()); t != "" {
				labels = append(labels, t)
			}
		})
		tr.Find("td").Each(func(_ int, td *goquery.Selection) {
			// Skip cells that only hold scripts/attachment widgets; their
			// text is captured separately via parseAttachments.
			if td.Find("script").Length() > 0 {
				return
			}
			if t := cleanText(td.Text()); t != "" {
				values = append(values, t)
			}
		})
		if len(labels) == 0 && len(values) == 0 {
			return
		}
		fields = append(fields, Field{Labels: labels, Values: values})
	})
	return fields
}

// parseAttachments extracts every downloadable file on a view.do page. Two
// markup forms exist: results-style boards reference files via an
// onclick="view('id','sn','bbs','key')" call, while data/notice boards link
// FileDown.do directly with the same params as a query string (no bbsKey).
// Both are handled.
func parseAttachments(doc *goquery.Document) []Attachment {
	var out []Attachment
	seen := map[string]bool{}
	add := func(name, id, sn, bbs, key string) {
		if id == "" {
			return
		}
		k := id + "|" + sn
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Attachment{
			Name:       cleanText(name),
			AtchFileID: id,
			FileSn:     sn,
			BbsID:      bbs,
			BbsKey:     key,
		})
	}
	// onclick view('atchFileId','fileSn','bbsId','bbsKey')
	doc.Find("a[onclick]").Each(func(_ int, a *goquery.Selection) {
		if m := reViewCall.FindStringSubmatch(mustAttr(a, "onclick")); m != nil {
			add(a.Text(), m[1], m[2], m[3], m[4])
		}
	})
	// direct href to FileDown.do?atchFileId=...&fileSn=...&bbsId=...
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := mustAttr(a, "href")
		if !strings.Contains(href, "FileDown.do") {
			return
		}
		q := parseRawQuery(href)
		add(a.Text(), q["atchFileId"], q["fileSn"], q["bbsId"], q["bbsKey"])
	})
	return out
}

func mustAttr(s *goquery.Selection, name string) string {
	v, _ := s.Attr(name)
	return v
}

// parseRawQuery splits a query string WITHOUT percent-decoding. atchFileId and
// fileSn arrive already-encoded and must stay verbatim for DownloadURL (see the
// FileDown.do encoding note in CLAUDE.md), so url.Values must not be used here.
func parseRawQuery(href string) map[string]string {
	if i := strings.IndexByte(href, '?'); i >= 0 {
		href = href[i+1:]
	}
	out := map[string]string{}
	for _, pair := range strings.Split(href, "&") {
		if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
			out[kv[0]] = kv[1]
		}
	}
	return out
}
