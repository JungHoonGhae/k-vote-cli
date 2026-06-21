package nesdc

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DownloadURL returns the absolute FileDown.do URL for an attachment. The
// atchFileId/fileSn/bbsKey values are already percent-encoded as they appear in
// the page markup, so they are concatenated verbatim (no re-encoding).
func (c *Client) DownloadURL(a Attachment) string {
	return fmt.Sprintf("%s/cmm/fms/FileDown.do?atchFileId=%s&fileSn=%s&bbsId=%s&bbsKey=%s",
		c.baseURL, a.AtchFileID, a.FileSn, a.BbsID, a.BbsKey)
}

var reCDFilename = regexp.MustCompile(`(?i)filename\*?=(?:UTF-8'')?"?([^";]+)"?`)

// Download fetches an attachment into destDir and returns the written path. The
// server-supplied filename (from Content-Disposition) is used when available,
// otherwise the attachment's display name is used.
//
// Some attachments are embargoed until the survey's scheduled publication time;
// for those the server returns an HTML notice instead of a file, which surfaces
// here as an error.
func (c *Client) Download(ctx context.Context, a Attachment, destDir string) (string, error) {
	c.throttle()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.DownloadURL(a), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", a.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", a.Name, resp.Status)
	}

	cd := resp.Header.Get("Content-Disposition")
	if cd == "" || !strings.Contains(strings.ToLower(cd), "attachment") {
		return "", fmt.Errorf("download %s: not a file response (possibly embargoed until publication)", a.Name)
	}

	name := filenameFromCD(cd)
	if name == "" {
		name = a.Name
	}
	name = sanitizeFilename(name)
	if name == "" {
		name = "attachment_" + a.FileSn
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, name)
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}
	return dest, nil
}

// filenameFromCD extracts and percent-decodes the filename from a
// Content-Disposition header. The portal encodes Korean filenames as
// percent-encoded UTF-8 with '+' for spaces.
func filenameFromCD(cd string) string {
	if _, params, err := mime.ParseMediaType(cd); err == nil {
		if fn := params["filename*"]; fn != "" {
			return decodeFilename(stripRFC5987(fn))
		}
		if fn := params["filename"]; fn != "" {
			return decodeFilename(fn)
		}
	}
	if m := reCDFilename.FindStringSubmatch(cd); m != nil {
		return decodeFilename(stripRFC5987(m[1]))
	}
	return ""
}

// stripRFC5987 removes an optional "UTF-8”" prefix from a filename* value.
func stripRFC5987(s string) string {
	if i := strings.Index(s, "''"); i >= 0 {
		return s[i+2:]
	}
	return s
}

// decodeFilename normalizes a Content-Disposition filename into proper UTF-8.
// The portal sends filenames in one of two broken-for-Go forms:
//   - percent-encoded UTF-8 with '+' for spaces, or
//   - raw UTF-8 bytes, which Go's HTTP layer decodes as Latin-1.
//
// Both are handled here.
func decodeFilename(s string) string {
	if strings.Contains(s, "%") {
		s = strings.ReplaceAll(s, "+", " ")
		s = percentDecode(s)
	}
	s = fixLatin1UTF8(s)
	return strings.TrimSpace(s)
}

// fixLatin1UTF8 repairs UTF-8 bytes that were mis-decoded as Latin-1: if every
// rune fits in a byte and the resulting byte slice is valid UTF-8, it is
// reinterpreted. Otherwise the input is returned unchanged.
func fixLatin1UTF8(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return s
		}
		b = append(b, byte(r))
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return s
}

// percentDecode decodes %XX sequences, leaving the input unchanged on error.
func percentDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if h := hexVal(s[i+1]); h >= 0 {
				if l := hexVal(s[i+2]); l >= 0 {
					b.WriteByte(byte(h<<4 | l))
					i += 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

var reUnsafe = regexp.MustCompile(`[/\\:*?"<>|]`)

// sanitizeFilename strips path separators and characters unsafe on common
// filesystems.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\x00", "")
	name = reUnsafe.ReplaceAllString(name, "_")
	return strings.Trim(name, ". ")
}
