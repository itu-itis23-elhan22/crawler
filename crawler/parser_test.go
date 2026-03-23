package crawler

import (
	"strings"
	"testing"
)

// TestExtractTitle verifies title extraction from various HTML inputs.
func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "simple title",
			html:     "<html><head><title>Hello World</title></head></html>",
			expected: "Hello World",
		},
		{
			name:     "title with whitespace",
			html:     "<title>  Trim Me  </title>",
			expected: "Trim Me",
		},
		{
			name:     "title with HTML entity",
			html:     "<title>AT&amp;T News</title>",
			expected: "AT&T News",
		},
		{
			name:     "uppercase TITLE tag",
			html:     "<TITLE>Upper Case</TITLE>",
			expected: "Upper Case",
		},
		{
			name:     "no title tag",
			html:     "<html><body><p>No title here</p></body></html>",
			expected: "",
		},
		{
			name:     "empty title",
			html:     "<title></title>",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitle(tt.html)
			if got != tt.expected {
				t.Errorf("extractTitle(%q) = %q; want %q", tt.html, got, tt.expected)
			}
		})
	}
}

// TestExtractLinks verifies that links are properly extracted and resolved.
func TestExtractLinks(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		baseURL  string
		expected []string // substrings that must appear in results
		excluded []string // substrings that must NOT appear in results
	}{
		{
			name:     "absolute http link",
			html:     `<a href="https://example.com/page">link</a>`,
			baseURL:  "https://example.com",
			expected: []string{"https://example.com/page"},
		},
		{
			name:     "relative link resolved against base",
			html:     `<a href="/about">About</a>`,
			baseURL:  "https://example.com",
			expected: []string{"https://example.com/about"},
		},
		{
			name:     "fragment-only link skipped",
			html:     `<a href="#section">Jump</a>`,
			baseURL:  "https://example.com",
			excluded: []string{"#section"},
		},
		{
			name:     "javascript link skipped",
			html:     `<a href="javascript:void(0)">Click</a>`,
			baseURL:  "https://example.com",
			excluded: []string{"javascript:"},
		},
		{
			name:     "mailto link skipped",
			html:     `<a href="mailto:test@example.com">Mail</a>`,
			baseURL:  "https://example.com",
			excluded: []string{"mailto:"},
		},
		{
			name:    "duplicate links deduplicated",
			html:    `<a href="/page">A</a><a href="/page">B</a>`,
			baseURL: "https://example.com",
		},
		{
			name:     "fragment removed from absolute URL",
			html:     `<a href="https://example.com/page#section">link</a>`,
			baseURL:  "https://example.com",
			expected: []string{"https://example.com/page"},
			excluded: []string{"#section"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := extractLinks(tt.html, tt.baseURL)

			for _, want := range tt.expected {
				found := false
				for _, link := range links {
					if link == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected link %q not found in %v", want, links)
				}
			}

			for _, unwanted := range tt.excluded {
				for _, link := range links {
					if strings.Contains(link, unwanted) {
						t.Errorf("excluded pattern %q found in link %q", unwanted, link)
					}
				}
			}

			// Check no duplicates for the duplicate test
			if tt.name == "duplicate links deduplicated" {
				seen := make(map[string]int)
				for _, link := range links {
					seen[link]++
				}
				for link, count := range seen {
					if count > 1 {
						t.Errorf("duplicate link found: %q appears %d times", link, count)
					}
				}
			}
		})
	}
}

// TestExtractWords verifies word extraction from HTML content.
func TestExtractWords(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		mustContain  []string
		mustNotHave  []string
		minWordCount int
	}{
		{
			name:        "basic word extraction",
			html:        "<p>Hello World Go Programming</p>",
			mustContain: []string{"hello", "world", "go", "programming"},
		},
		{
			name:        "script tags removed",
			html:        `<p>visible</p><script>var secret = "hidden";</script>`,
			mustContain: []string{"visible"},
			mustNotHave: []string{"secret", "hidden", "var"},
		},
		{
			name:        "style tags removed",
			html:        `<p>content</p><style>.hidden { color: red; }</style>`,
			mustContain: []string{"content"},
			mustNotHave: []string{"color", "hidden"},
		},
		{
			name:        "HTML tags stripped",
			html:        `<div class="main"><span>extracted</span></div>`,
			mustContain: []string{"extracted"},
			mustNotHave: []string{"div", "class", "main", "span"},
		},
		{
			name:        "words lowercased",
			html:        "<p>GoLang CRAWLER Test</p>",
			mustContain: []string{"golang", "crawler", "test"},
		},
		{
			name:        "single chars filtered out",
			html:        "<p>a b c word</p>",
			mustContain: []string{"word"},
			mustNotHave: []string{"a", "b", "c"},
		},
		{
			// &amp; decodes to &, then "AT&T" splits into ["at", "t"].
			// Single-char words are filtered out (len >= 2 rule), so only "at" remains.
			name:        "HTML entities decoded",
			html:        "<p>AT&amp;T corporation</p>",
			mustContain: []string{"at", "corporation"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := extractWords(tt.html)

			wordSet := make(map[string]bool, len(words))
			for _, w := range words {
				wordSet[w] = true
			}

			for _, want := range tt.mustContain {
				if !wordSet[want] {
					t.Errorf("expected word %q not found in %v", want, words)
				}
			}

			for _, unwanted := range tt.mustNotHave {
				if wordSet[unwanted] {
					t.Errorf("unwanted word %q found in result", unwanted)
				}
			}

			if tt.minWordCount > 0 && len(words) < tt.minWordCount {
				t.Errorf("expected at least %d words, got %d", tt.minWordCount, len(words))
			}
		})
	}
}

// TestDecodeBasicEntities verifies HTML entity decoding.
func TestDecodeBasicEntities(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"AT&amp;T", "AT&T"},
		{"&lt;div&gt;", "<div>"},
		{"&quot;hello&quot;", `"hello"`},
		{"&#39;apostrophe&#39;", "'apostrophe'"},
		{"hello&nbsp;world", "hello world"},
		{"no entities here", "no entities here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := decodeBasicEntities(tt.input)
			if got != tt.expected {
				t.Errorf("decodeBasicEntities(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestParseHTML verifies the top-level ParseHTML function integrates all parts.
func TestParseHTML(t *testing.T) {
	html := `<html>
<head><title>Go Crawler Test</title></head>
<body>
  <p>This is a web crawler built with Go.</p>
  <a href="https://golang.org">Golang</a>
  <a href="/about">About</a>
  <script>console.log("ignored");</script>
</body>
</html>`

	result := ParseHTML(html, "https://example.com", "https://example.com", 0)

	if result.Title != "Go Crawler Test" {
		t.Errorf("Title = %q; want %q", result.Title, "Go Crawler Test")
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q; want %q", result.URL, "https://example.com")
	}
	if result.Origin != "https://example.com" {
		t.Errorf("Origin = %q; want %q", result.Origin, "https://example.com")
	}
	if result.Depth != 0 {
		t.Errorf("Depth = %d; want 0", result.Depth)
	}

	// Check links
	foundGolang := false
	for _, link := range result.Links {
		if link == "https://golang.org" {
			foundGolang = true
		}
	}
	if !foundGolang {
		t.Errorf("expected golang.org in links, got: %v", result.Links)
	}

	// Check words — "crawler" and "golang" should appear
	wordSet := make(map[string]bool)
	for _, w := range result.Words {
		wordSet[w] = true
	}
	for _, expected := range []string{"crawler", "web", "built"} {
		if !wordSet[expected] {
			t.Errorf("expected word %q in words, got: %v", expected, result.Words)
		}
	}

	// "ignored" from script should NOT appear
	if wordSet["ignored"] {
		t.Errorf("script content 'ignored' should not appear in words")
	}
}
