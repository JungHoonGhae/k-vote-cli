package nec

import (
	"mime"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	reCDFilename = regexp.MustCompile(`(?i)filename\*?=(?:UTF-8'')?"?([^";]+)"?`)
	reUnsafe     = regexp.MustCompile(`[/\\:*?"<>|]`)
)

// filenameFromCD extracts and normalizes a filename from a Content-Disposition
// header, handling RFC 5987 (filename*) and the percent / Latin-1 encodings the
// portal occasionally emits.
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

func stripRFC5987(s string) string {
	if i := strings.LastIndex(s, "''"); i >= 0 {
		return s[i+2:]
	}
	return s
}

func decodeFilename(s string) string {
	if strings.Contains(s, "%") {
		s = percentDecode(s)
	}
	s = fixLatin1UTF8(s)
	return strings.TrimSpace(s)
}

// fixLatin1UTF8 repairs UTF-8 bytes mis-decoded as Latin-1.
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

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\x00", "")
	name = reUnsafe.ReplaceAllString(name, "_")
	return strings.Trim(name, ". ")
}
