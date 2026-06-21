package nec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Source selects which NEC backend a command targets.
type Source string

const (
	SourceDataGoKr   Source = "datagokr"   // data.go.kr (default)
	SourceOpenPortal Source = "openportal" // data.nec.go.kr 개방포털
)

// OpenPortalBaseURL is the NEC 국가선거정보 개방포털 root. It is HTTP-only (the
// HTTPS endpoint does not respond) and, unlike info.nec.go.kr, allows automated
// access (robots.txt: Allow: /). Files are free and unrestricted.
const OpenPortalBaseURL = "http://data.nec.go.kr"

// OPDataset is one open-portal dataset (개표결과/투표율/당선인/…) keyed by dataId.
type OPDataset struct {
	DataID string `json:"dataId"`
	Title  string `json:"title"`
}

// OPFile is one downloadable attachment of a dataset (회차/형식별로 여러 개).
type OPFile struct {
	AttachFileID string `json:"attachFileId"`
	Name         string `json:"name"` // 파일명 (확장자 포함)
}

var (
	reOPDataID = regexp.MustCompile(`dataId=(\d+)`)
	reOPAttach = regexp.MustCompile(`attachFileId=(\d+)`)
)

// openPortalDoc performs a throttled GET against the open portal and parses HTML.
func (c *Client) openPortalDoc(ctx context.Context, path string, query url.Values) (*goquery.Document, error) {
	resp, err := c.openPortalGet(ctx, path, query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GET %s: unexpected status %s", path, resp.Status)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

// openPortalGet is a throttled GET against c.opBaseURL. The caller owns the body.
func (c *Client) openPortalGet(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	c.throttle()
	u := c.opBaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,*/*")
	req.Header.Set("Referer", c.opBaseURL+"/")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	return resp, nil
}

// OpenPortalDatasets searches the open-portal catalog. A blank keyword lists the
// first page. Dataset titles live in the link's title attribute, not its text.
func (c *Client) OpenPortalDatasets(ctx context.Context, keyword string) ([]OPDataset, error) {
	q := url.Values{}
	q.Set("keyword", keyword)
	q.Set("openDataType", "ALL")
	doc, err := c.openPortalDoc(ctx, "/open-data/data-list.do", q)
	if err != nil {
		return nil, err
	}
	var out []OPDataset
	seen := map[string]bool{}
	// Match by query param, not exact path: the portal injects ";jsessionid=…"
	// between "file.do" and "?dataId=" on cookieless requests, which would break
	// an adjacency selector.
	doc.Find(`a[href*="dataId="]`).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if !strings.Contains(href, "/open-data/file.do") {
			return
		}
		m := reOPDataID.FindStringSubmatch(href)
		if m == nil || seen[m[1]] {
			return
		}
		title := cleanText(attrOf(a, "title"))
		if title == "" {
			title = cleanText(a.Text())
		}
		seen[m[1]] = true
		out = append(out, OPDataset{DataID: m[1], Title: title})
	})
	return out, nil
}

// OpenPortalFiles lists a dataset's downloadable attachments. The filename is in
// each download link's title attribute (e.g. "제21대 대통령선거 개표결과.xlsx 다운로드").
func (c *Client) OpenPortalFiles(ctx context.Context, dataID string) ([]OPFile, error) {
	q := url.Values{}
	q.Set("dataId", dataID)
	doc, err := c.openPortalDoc(ctx, "/open-data/file.do", q)
	if err != nil {
		return nil, err
	}
	var out []OPFile
	seen := map[string]bool{}
	doc.Find(`a[href*="attachFileId="]`).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if !strings.Contains(href, "file-download.do") {
			return
		}
		m := reOPAttach.FindStringSubmatch(href)
		if m == nil || seen[m[1]] {
			return
		}
		name := cleanText(attrOf(a, "title"))
		name = strings.TrimSpace(strings.TrimSuffix(name, "다운로드"))
		seen[m[1]] = true
		out = append(out, OPFile{AttachFileID: m[1], Name: name})
	})
	return out, nil
}

// OpenPortalDownload streams an attachment into destDir, returning the written
// path. The server-supplied Content-Disposition filename is preferred.
func (c *Client) OpenPortalDownload(ctx context.Context, attachFileID, destDir string) (string, error) {
	q := url.Values{}
	q.Set("attachFileId", attachFileID)
	resp, err := c.openPortalGet(ctx, "/file-download.do", q)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download attachFileId=%s: unexpected status %s", attachFileID, resp.Status)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(strings.ToLower(cd), "attachment") {
		return "", fmt.Errorf("download attachFileId=%s: not a file response", attachFileID)
	}
	name := sanitizeFilename(filenameFromCD(cd))
	if name == "" {
		name = "attach-" + attachFileID
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(destDir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

// PickXLSX returns the first attachment whose name ends in .xlsx (the open portal
// distributes 개표결과 as XLSX, which ParseResultsXLSX handles). Falls back to the
// first .csv, else the first file.
func PickXLSX(files []OPFile) (OPFile, bool) {
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f.Name), ".xlsx") {
			return f, true
		}
	}
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			return f, true
		}
	}
	if len(files) > 0 {
		return files[0], true
	}
	return OPFile{}, false
}

func attrOf(s *goquery.Selection, name string) string {
	v, _ := s.Attr(name)
	return v
}
