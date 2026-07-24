package parser

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// runWithTimeout runs fn and fails the test if it does not return in time.
// This guards against the regression where scanTagAttrs could loop forever.
func runWithTimeout(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("ExtractLinks timed out (possible infinite loop in parser)")
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

func TestNormalizeRelativeResolution(t *testing.T) {
	base := mustParse(t, "https://www.example.com/sub/page")

	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"/about", "https://www.example.com/about", true},
		{"contact.html", "https://www.example.com/sub/contact.html", true},
		{"../up.html", "https://www.example.com/up.html", true},
		{"./here.html", "https://www.example.com/sub/here.html", true},
		{"?q=1", "https://www.example.com/sub/page?q=1", true},
		{"/p?q=1#frag", "https://www.example.com/p?q=1", true},               // fragment stripped
		{"#section", "", false},                                              // fragment-only rejected
		{"", "", false},                                                      // empty rejected
		{"mailto:foo@bar.com", "", false},                                    // non-http(s) rejected
		{"https://other.example.com/x", "https://other.example.com/x", true}, // external allowed here; filtered later by sameHost
		{"//cdn.example.com/a", "https://cdn.example.com/a", true},           // protocol-relative resolved to https
	}
	for _, c := range cases {
		got, ok := normalize(base, c.raw)
		if ok != c.ok {
			t.Errorf("normalize(%q) ok=%v want %v", c.raw, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.String() != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.raw, got.String(), c.want)
		}
	}
}

func TestSameHost(t *testing.T) {
	base := mustParse(t, "https://www.example.com/")
	cases := []struct {
		raw string
		got bool
	}{
		{"https://www.example.com/a", true},
		{"https://WWW.EXAMPLE.com/b", true}, // case-insensitive host
		{"https://api.example.com/x", false},
		{"https://example.com/x", false}, // different subdomain
	}
	for _, c := range cases {
		u := mustParse(t, c.raw)
		if got := sameHost(base, u); got != c.got {
			t.Errorf("sameHost(%q) = %v, want %v", c.raw, got, c.got)
		}
	}
}

func TestExtractLinksResolvesRelative(t *testing.T) {
	html := `<a href="/about">A</a><a href="img.png">I</a><a href="../up">U</a>`
	base := mustParse(t, "https://www.example.com/dir/page")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{
		"https://www.example.com/about":       true,
		"https://www.example.com/dir/img.png": true,
		"https://www.example.com/up":          true,
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

func TestExtractLinksSkipsExternalAndNonHTTP(t *testing.T) {
	html := `<a href="https://other.com/x">E</a><a href="mailto:a@b.com">M</a><a href="ftp://x/y">F</a><a href="/local">L</a>`
	base := mustParse(t, "https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 1 || links[0] != "https://www.example.com/local" {
		t.Fatalf("got %v, want [https://www.example.com/local]", links)
	}
}

func TestExtractLinksDeduplicates(t *testing.T) {
	html := `<a href="/a">A</a><a href="/a">A2</a><a href="/a">A3</a>`
	base := mustParse(t, "https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1 (deduped): %v", len(links), links)
	}
}

func TestExtractLinksIgnoresNonAnchorTags(t *testing.T) {
	html := `<link rel="stylesheet" href="/style.css">` +
		`<script src="/app.js"></script>` +
		`<img src="/pic.png" />` +
		`<a href="/page">P</a>`
	base := mustParse(t, "https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 1 || links[0] != "https://www.example.com/page" {
		t.Fatalf("got %v, want only the anchor link", links)
	}
}

// TestExtractLinksNoInfiniteLoop is a regression test for the parser bug where
// scanTagAttrs could loop forever on tags whose final attribute was followed
// immediately by '>' or '/' with no '='. The inputs below previously hung.
func TestExtractLinksNoInfiniteLoop(t *testing.T) {
	base := mustParse(t, "https://www.example.com/")
	inputs := []string{
		`<a href="/x">`,
		`<a href='/x'>`,
		`<a href="/x" />`,
		`<option selected><a href="/y">Y</a></option>`,
		`<br/><a href="/z">Z</a>`,
		`<a href="/a" class="b" id="c">A</a>`,
		`<a href="/d" data-x="1" />`,
		`<a href="/e"></a><a href="/f"></a>`,
	}
	for _, in := range inputs {
		in := in
		runWithTimeout(t, 2*time.Second, func() {
			links, err := ExtractLinks(base, strings.NewReader(in))
			if err != nil {
				t.Errorf("input %q: unexpected error: %v", in, err)
			}
			// Every input contains at least one internal anchor link.
			if len(links) == 0 {
				t.Errorf("input %q: expected at least one link, got none", in)
			}
		})
	}
}

func TestExtractLinksHandlesQuotesAndSpaces(t *testing.T) {
	html := `<a   href =  "/spaced"  >S</a><a href="/dbl" class="x">D</a><a href='/sng'>S2</a>`
	base := mustParse(t, "https://www.example.com/")
	links, err := ExtractLinks(base, strings.NewReader(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{
		"https://www.example.com/spaced": true,
		"https://www.example.com/dbl":    true,
		"https://www.example.com/sng":    true,
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
