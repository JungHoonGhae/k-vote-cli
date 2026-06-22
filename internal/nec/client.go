// Package nec implements a keyless client for NEC (중앙선거관리위원회) election
// results that are published as downloadable file datasets on the public data
// portal data.go.kr.
//
// The NEC election-statistics site (info.nec.go.kr) actively blocks automated
// access (robots.txt Disallow: /, server-side rejection of programmatic
// requests), so kvote deliberately does not scrape it. Instead it uses the
// official open-data distribution channel: the NEC uploads 개표결과·투표율
// file datasets (CSV/XLSX) to data.go.kr, where the file download itself
// requires no API key. This is a sanctioned, polite path to the same numbers.
package nec

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DefaultBaseURL is the public data portal root.
const DefaultBaseURL = "https://www.data.go.kr"

// DefaultUserAgent identifies the client honestly to the server operator.
const DefaultUserAgent = "kvote-cli (+https://github.com/JungHoonGhae/kvote-cli)"

// DefaultDelay is the minimum spacing between requests.
const DefaultDelay = 700 * time.Millisecond

// DefaultOrg scopes dataset searches to the NEC by publisher name.
const DefaultOrg = "중앙선거관리위원회"

// Client is a rate-limited HTTP client for NEC file datasets. It speaks to two
// backends: data.go.kr (default) and the NEC open portal data.nec.go.kr.
type Client struct {
	baseURL    string // data.go.kr
	opBaseURL  string // data.nec.go.kr 개방포털
	apiBaseURL string // apis.data.go.kr OpenAPI 게이트웨이
	userAgent  string
	delay      time.Duration
	http       *http.Client

	mu      sync.Mutex
	lastReq time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the data.go.kr root (useful for tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithOpenPortalBaseURL overrides the data.nec.go.kr open-portal root (tests).
func WithOpenPortalBaseURL(u string) Option {
	return func(c *Client) { c.opBaseURL = strings.TrimRight(u, "/") }
}

// WithAPIBaseURL overrides the apis.data.go.kr OpenAPI gateway root (tests).
func WithAPIBaseURL(u string) Option {
	return func(c *Client) { c.apiBaseURL = strings.TrimRight(u, "/") }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithDelay sets the minimum spacing between requests.
func WithDelay(d time.Duration) Option {
	return func(c *Client) {
		if d >= 0 {
			c.delay = d
		}
	}
}

// WithHTTPClient injects a custom *http.Client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New creates a Client with sane defaults.
func New(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		opBaseURL:  OpenPortalBaseURL,
		apiBaseURL: APIBaseURL,
		userAgent:  DefaultUserAgent,
		delay:      DefaultDelay,
		http:       &http.Client{Timeout: 120 * time.Second}, // dataset files can be large
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// throttle blocks until at least c.delay has passed since the previous request.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.delay > 0 && !c.lastReq.IsZero() {
		if wait := c.delay - time.Since(c.lastReq); wait > 0 {
			time.Sleep(wait)
		}
	}
	c.lastReq = time.Now()
}

// rawGet performs a throttled GET and returns the response. The caller owns the body.
func (c *Client) rawGet(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	c.throttle()

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/json,*/*")
	req.Header.Set("Referer", c.baseURL+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	return resp, nil
}

// getDoc performs a throttled GET and parses the response as HTML.
func (c *Client) getDoc(ctx context.Context, path string, query url.Values) (*goquery.Document, error) {
	resp, err := c.rawGet(ctx, path, query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GET %s: unexpected status %s", path, resp.Status)
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, nil
}
