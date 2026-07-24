package parser

import (
	"net/url"
	"strings"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	html := `
		<html><body>
		<a href="/about">About</a>
		<a href="/about">About dup</a>
		<a href="contact.html">Contact</a>
		<a href="https://other.example.com/x">External</a>
		<a href="#section">Fragment</a>
		<a href="mailto:foo@bar.com">Mail</a>
		<a href="/p?q=1#frag">Query</a>
		</body></html>`

	base, _ := url.Parse("https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{
		"https://www.example.com/about":        true,
		"https://www.example.com/contact.html": true,
		"https://www.example.com/p?q=1":        true,
	}
	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d: %v", len(links), len(want), links)
	}
	for _, l := range links {
		if !want[l] {
			t.Errorf("unexpected link: %s", l)
		}
	}
}

func TestExtractLinksIgnoresNonAnchor(t *testing.T) {
	html := `<link rel="stylesheet" href="/style.css"><script src="/app.js"></script><a href="/page">P</a>`
	base, _ := url.Parse("https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 1 || links[0] != "https://www.example.com/page" {
		t.Fatalf("expected only the anchor link, got %v", links)
	}
}
