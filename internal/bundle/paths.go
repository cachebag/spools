package bundle

import (
	"bytes"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

// Placeholders stand in for machine-specific paths inside a payload. The _URI
// forms mark paths that appeared inside file:// URIs, which are escaped
// differently from plain JSON strings.
const (
	Project    = "{{PROJECT}}"
	ProjectURI = "{{PROJECT_URI}}"
	Home       = "{{HOME}}"
	HomeURI    = "{{HOME_URI}}"
)

// Templatize swaps root and home for placeholders in a JSON payload. root goes
// first since it usually lives under home.
func Templatize(payload []byte, root, home string) []byte {
	s := string(payload)
	for _, p := range []struct{ path, uri, plain string }{
		{root, ProjectURI, Project},
		{home, HomeURI, Home},
	} {
		if p.path == "" {
			continue
		}
		s = replacePath(s, "file://"+uriPath(p.path), "file://"+p.uri)
		s = replacePath(s, jsonString(p.path), p.plain)
	}
	return []byte(s)
}

// Expand is the inverse of Templatize, filling placeholders with this
// machine's root and home.
func Expand(payload []byte, root, home string) []byte {
	return []byte(strings.NewReplacer(
		ProjectURI, uriPath(root),
		Project, jsonString(root),
		HomeURI, uriPath(home),
		Home, jsonString(home),
	).Replace(string(payload)))
}

// replacePath replaces old with new only where old ends at a path boundary,
// so /code/spools doesn't clobber /code/spools-web.
func replacePath(s, old, new string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(old)
		b.WriteString(s[:i])
		if end < len(s) && isNameByte(s[end]) {
			b.WriteString(old)
		} else {
			b.WriteString(new)
		}
		s = s[end:]
	}
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
}

// jsonString is how path appears inside a JSON string literal (backslashes
// doubled on windows, etc).
func jsonString(path string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(path)
	return strings.TrimSuffix(strings.TrimSuffix(buf.String(), "\n"), `"`)[1:]
}

// uriPath is how path appears after file:// in a URI.
func uriPath(path string) string {
	if path == "" {
		return ""
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Path: p}).EscapedPath()
}
