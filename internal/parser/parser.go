// Package parser extracts internal links from HTML documents.
//
// It is intentionally minimal: it discovers <a href> attributes and returns
// them normalized to absolute, same-host URLs. No SEO, security, or DOM
// analysis is performed.
package parser

import (
	"bytes"
	"io"
	"net/url"
	"strings"
)

// ExtractLinks reads an HTML document from r and returns the internal links
// (same host as base) discovered via <a href> attributes. Returned links are
// absolute, fragment-stripped, and deduplicated.
func ExtractLinks(base *url.URL, r io.Reader) ([]string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	raw := extractAttrValues(body, "href")

	seen := make(map[string]bool)
	var links []string
	for _, v := range raw {
		u, ok := normalize(base, v)
		if !ok {
			continue
		}
		if !sameHost(base, u) {
			continue
		}
		key := u.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		links = append(links, key)
	}
	return links, nil
}

// extractAttrValues scans raw HTML and returns the values of the named
// attribute (e.g. "href") found inside <a> tags only. Links from other tags
// such as <link> or <script> are ignored, per the crawler's contract of
// discovering internal navigation links.
func extractAttrValues(html []byte, attr string) []string {
	var vals []string
	n := len(html)
	i := 0
	attrLower := []byte(strings.ToLower(attr))
	for i < n {
		for i < n && html[i] != '<' {
			i++
		}
		if i >= n {
			break
		}
		j := i + 1
		for j < n && html[j] != '>' {
			j++
		}
		if j >= n {
			break
		}
		tag := html[i+1 : j]
		// Only consider anchor tags.
		if !bytes.HasPrefix(bytes.TrimLeft(tag, " \t\n\r"), []byte("a ")) &&
			!bytes.HasPrefix(bytes.TrimLeft(tag, " \t\n\r"), []byte("a>")) &&
			!bytes.HasPrefix(bytes.TrimLeft(tag, " \t\n\r"), []byte("a/")) {
			i = j + 1
			continue
		}
		vals = append(vals, scanTagAttrs(tag, attrLower)...)
		i = j + 1
	}
	return vals
}

// scanTagAttrs extracts attribute values from a single tag's inner bytes.
func scanTagAttrs(tag, attr []byte) []string {
	var vals []string
	k := 0
	lt := len(tag)
	for k < lt {
		for k < lt && isSpace(tag[k]) {
			k++
		}
		if k >= lt {
			break
		}
		// A '>' or '/' terminates the tag; consume it and move on.
		if tag[k] == '>' || tag[k] == '/' {
			k++
			continue
		}
		nameStart := k
		for k < lt && !isSpace(tag[k]) && tag[k] != '=' && tag[k] != '/' && tag[k] != '>' {
			k++
		}
		name := tag[nameStart:k]
		for k < lt && isSpace(tag[k]) {
			k++
		}
		if k < lt && tag[k] == '=' {
			k++
			for k < lt && isSpace(tag[k]) {
				k++
			}
			var val string
			if k < lt && (tag[k] == '"' || tag[k] == '\'') {
				quote := tag[k]
				k++
				vs := k
				for k < lt && tag[k] != quote {
					k++
				}
				val = string(tag[vs:k])
				if k < lt {
					k++
				}
			} else {
				vs := k
				for k < lt && !isSpace(tag[k]) && tag[k] != '>' && tag[k] != '/' {
					k++
				}
				val = string(tag[vs:k])
			}
			if bytes.EqualFold(name, attr) {
				vals = append(vals, val)
			}
		} else if bytes.EqualFold(name, attr) {
			// Boolean attribute (no value), e.g. <option selected>.
			vals = append(vals, "")
		}
	}
	return vals
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// normalize resolves raw against base, rejects non-http(s) schemes, and strips
// the fragment. Fragment-only hrefs (e.g. "#section") point at the current
// page and are rejected.
func normalize(base *url.URL, raw string) (*url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	u = base.ResolveReference(u)
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	u.Fragment = ""
	return u, true
}

func sameHost(base, u *url.URL) bool {
	return strings.EqualFold(u.Host, base.Host)
}
