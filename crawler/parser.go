package crawler

import (
	"net/url"
	"regexp"
	"strings"

	"crawler/models"
)

// Regular expressions compiled once and reused (performance optimization)
var (
	// Finds all <a href="..."> tags in HTML
	linkRegex = regexp.MustCompile(`<a\s+[^>]*href\s*=\s*["']([^"']+)["']`)

	// Finds the <title>...</title> content
	titleRegex = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)

	// Matches HTML tags like <div>, </p>, <br/> etc.
	tagRegex = regexp.MustCompile(`<[^>]+>`)

	// Matches <script>...</script> blocks (including content)
	scriptRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)

	// Matches <style>...</style> blocks (including content)
	styleRegex = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)

	// Matches non-letter/non-number characters (for splitting words)
	nonWordRegex = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// ParseHTML takes raw HTML and the page's URL, extracts links, title, and words.
// Returns a PageData struct with all extracted information.
func ParseHTML(rawHTML string, pageURL string, origin string, depth int) models.PageData {
	result := models.PageData{
		URL:    pageURL,
		Origin: origin,
		Depth:  depth,
	}

	// Extract title
	result.Title = extractTitle(rawHTML)

	// Extract links and resolve them to absolute URLs
	result.Links = extractLinks(rawHTML, pageURL)

	// Extract words from visible text
	result.Words = extractWords(rawHTML)

	return result
}

// extractTitle pulls the text inside <title>...</title>.
func extractTitle(html string) string {
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		// Remove any HTML entities
		title = decodeBasicEntities(title)
		return title
	}
	return ""
}

// extractLinks finds all href values in <a> tags and resolves relative URLs.
func extractLinks(html string, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	matches := linkRegex.FindAllStringSubmatch(html, -1)
	var links []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		href := strings.TrimSpace(match[1])

		// Skip empty links, fragments, javascript, and mailto
		if href == "" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "javascript:") ||
			strings.HasPrefix(href, "mailto:") {
			continue
		}

		// Resolve relative URL to absolute
		resolved, err := base.Parse(href)
		if err != nil {
			continue
		}

		// Only keep http and https links
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			continue
		}

		// Remove fragment (#section) part — same page different section
		resolved.Fragment = ""

		absoluteURL := resolved.String()

		// Deduplicate within this page's links
		if !seen[absoluteURL] {
			seen[absoluteURL] = true
			links = append(links, absoluteURL)
		}
	}

	return links
}

// extractWords gets all visible text from HTML and splits into lowercase words.
func extractWords(html string) []string {
	// Step 1: Remove <script> blocks — they contain code, not readable text
	clean := scriptRegex.ReplaceAllString(html, " ")

	// Step 2: Remove <style> blocks — CSS is not readable text
	clean = styleRegex.ReplaceAllString(clean, " ")

	// Step 3: Remove all remaining HTML tags
	clean = tagRegex.ReplaceAllString(clean, " ")

	// Step 4: Decode common HTML entities
	clean = decodeBasicEntities(clean)

	// Step 5: Convert to lowercase
	clean = strings.ToLower(clean)

	// Step 6: Split by non-word characters
	rawWords := nonWordRegex.Split(clean, -1)

	// Step 7: Filter out empty strings and very short words
	var words []string
	for _, word := range rawWords {
		if len(word) >= 2 { // Skip single-character "words"
			words = append(words, word)
		}
	}

	return words
}

// decodeBasicEntities replaces common HTML entities with their characters.
func decodeBasicEntities(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return replacer.Replace(s)
}
