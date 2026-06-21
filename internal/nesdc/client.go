// Package nesdc implements an unofficial scraping client for the NESDC
// (중앙선거여론조사심의위원회 / National Election Survey Deliberation Commission)
// public portal at https://www.nesdc.go.kr.
//
// All data exposed here is legally mandated public disclosure under the
// 선거여론조사기준. The client is deliberately polite: requests are
// rate-limited and identify themselves via a descriptive User-Agent.
package nesdc

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

// DefaultBaseURL is the portal root used for every request.
const DefaultBaseURL = "https://www.nesdc.go.kr/portal"

// DefaultUserAgent identifies the client honestly to the server operator.
const DefaultUserAgent = "nesdc-cli (+https://github.com/JungHoonGhae/nesdc-cli)"

// DefaultDelay is the minimum spacing between requests.
const DefaultDelay = 700 * time.Millisecond

// Client is a rate-limited HTTP client for the NESDC portal. It is safe for
// concurrent use; requests are serialized through the rate limiter.
type Client struct {
	baseURL   string
	userAgent string
	delay     time.Duration
	http      *http.Client

	mu      sync.Mutex
	lastReq time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the portal root (useful for tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

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
		baseURL:   DefaultBaseURL,
		userAgent: DefaultUserAgent,
		delay:     DefaultDelay,
		http:      &http.Client{Timeout: 30 * time.Second},
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
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

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
