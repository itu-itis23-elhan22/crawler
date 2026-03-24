package index

import (
	"sync"
	"testing"

	"crawler/models"
)

// makePageData is a helper to create test PageData.
func makePageData(url, origin, title string, depth int, words []string) models.PageData {
	return models.PageData{
		URL:    url,
		Origin: origin,
		Title:  title,
		Depth:  depth,
		Words:  words,
	}
}

// TestAdd verifies that pages are correctly added to the index.
func TestAdd(t *testing.T) {
	idx := NewInvertedIndex()

	if idx.Size() != 0 {
		t.Fatalf("expected empty index, got size %d", idx.Size())
	}

	page := makePageData("https://example.com", "https://example.com", "Test Page", 0,
		[]string{"go", "crawler", "web", "crawler", "go"})
	idx.Add(page)

	// After adding one page with 3 unique words, size should be 3
	if idx.Size() != 3 {
		t.Errorf("expected size 3, got %d", idx.Size())
	}
}

// TestSearch verifies basic search functionality.
func TestSearch(t *testing.T) {
	idx := NewInvertedIndex()

	page1 := makePageData("https://go.dev", "https://go.dev", "The Go Language", 0,
		[]string{"go", "programming", "language", "concurrency"})
	page2 := makePageData("https://rust-lang.org", "https://rust-lang.org", "Rust Language", 0,
		[]string{"rust", "programming", "language", "memory", "safety"})
	idx.Add(page1)
	idx.Add(page2)

	// Search for "go" — should only return page1
	results := idx.Search("go")
	if len(results) == 0 {
		t.Fatal("expected results for 'go', got none")
	}
	found := false
	for _, r := range results {
		if r.RelevantURL == "https://go.dev" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected https://go.dev in results for 'go'")
	}

	// Search for common word "programming" — should return both pages
	results = idx.Search("programming")
	if len(results) < 2 {
		t.Errorf("expected at least 2 results for 'programming', got %d", len(results))
	}

	// Search for non-existent term
	results = idx.Search("doesnotexist12345")
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-existent term, got %d", len(results))
	}
}

// TestSearchMultiTerm verifies that multi-word queries work correctly.
func TestSearchMultiTerm(t *testing.T) {
	idx := NewInvertedIndex()

	page := makePageData("https://example.com", "https://example.com", "Go Web Crawler", 0,
		[]string{"go", "web", "crawler", "concurrent", "indexing"})
	idx.Add(page)

	results := idx.Search("go web crawler")
	if len(results) == 0 {
		t.Fatal("expected results for multi-term query, got none")
	}
	if results[0].RelevantURL != "https://example.com" {
		t.Errorf("expected https://example.com as top result, got %s", results[0].RelevantURL)
	}
}

// TestSearchReturnsTriples verifies the result structure (relevant_url, origin_url, depth).
func TestSearchReturnsTriples(t *testing.T) {
	idx := NewInvertedIndex()

	page := makePageData("https://page.com/article", "https://origin.com", "Article Title", 2,
		[]string{"article", "content", "text"})
	idx.Add(page)

	results := idx.Search("article")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.RelevantURL != "https://page.com/article" {
		t.Errorf("RelevantURL = %q; want %q", r.RelevantURL, "https://page.com/article")
	}
	if r.OriginURL != "https://origin.com" {
		t.Errorf("OriginURL = %q; want %q", r.OriginURL, "https://origin.com")
	}
	if r.Depth != 2 {
		t.Errorf("Depth = %d; want 2", r.Depth)
	}
	if r.Score <= 0 {
		t.Errorf("Score should be positive, got %f", r.Score)
	}
}

// TestSearchEmptyQuery verifies that an empty query returns no results.
func TestSearchEmptyQuery(t *testing.T) {
	idx := NewInvertedIndex()
	idx.Add(makePageData("https://example.com", "https://example.com", "Title", 0,
		[]string{"word", "another"}))

	results := idx.Search("")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}

	results = idx.Search("   ")
	if len(results) != 0 {
		t.Errorf("expected 0 results for whitespace-only query, got %d", len(results))
	}
}

// TestSearchScoreOrdering verifies that more relevant results rank higher.
func TestSearchScoreOrdering(t *testing.T) {
	idx := NewInvertedIndex()

	// page1: "go" appears 5 times out of 10 words = high TF
	page1 := makePageData("https://high.com", "https://high.com", "Go", 0,
		[]string{"go", "go", "go", "go", "go", "programming", "language", "fast", "safe", "fun"})

	// page2: "go" appears 1 time out of 10 words = lower TF
	page2 := makePageData("https://low.com", "https://low.com", "Other", 0,
		[]string{"go", "rust", "python", "java", "swift", "kotlin", "scala", "ruby", "elixir", "haskell"})

	idx.Add(page1)
	idx.Add(page2)

	results := idx.Search("go")
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].RelevantURL != "https://high.com" {
		t.Errorf("expected high.com to rank first, got %s", results[0].RelevantURL)
	}
}

// TestTitleBoost verifies that terms in the title score higher.
func TestTitleBoost(t *testing.T) {
	idx := NewInvertedIndex()

	// page1: "golang" in title
	pageWithTitle := makePageData("https://with-title.com", "https://with-title.com",
		"Golang Programming", 0, []string{"golang", "programming", "tutorial"})

	// page2: "golang" only in body, same word count
	pageNoTitle := makePageData("https://no-title.com", "https://no-title.com",
		"Programming Tutorial", 0, []string{"golang", "programming", "tutorial"})

	idx.Add(pageWithTitle)
	idx.Add(pageNoTitle)

	results := idx.Search("golang")
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// The page with "golang" in the title should score higher
	if results[0].RelevantURL != "https://with-title.com" {
		t.Errorf("expected with-title.com to rank first due to title boost, got %s", results[0].RelevantURL)
	}
}

// TestSnapshotAndRestore verifies persistence round-trip.
func TestSnapshotAndRestore(t *testing.T) {
	original := NewInvertedIndex()
	original.Add(makePageData("https://a.com", "https://a.com", "Page A", 0,
		[]string{"alpha", "beta", "gamma"}))
	original.Add(makePageData("https://b.com", "https://b.com", "Page B", 1,
		[]string{"delta", "epsilon", "alpha"}))

	// Take snapshot
	snapshot := original.Snapshot()

	// Restore into a new index
	restored := NewInvertedIndex()
	restored.Restore(snapshot)

	if restored.Size() != original.Size() {
		t.Errorf("restored size %d != original size %d", restored.Size(), original.Size())
	}

	// Search should work in restored index
	results := restored.Search("alpha")
	if len(results) < 2 {
		t.Errorf("expected at least 2 results for 'alpha' in restored index, got %d", len(results))
	}
}

// TestConcurrentAddAndSearch verifies there are no data races during concurrent use.
func TestConcurrentAddAndSearch(t *testing.T) {
	idx := NewInvertedIndex()

	var wg sync.WaitGroup

	// 10 concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			page := makePageData(
				"https://example.com/"+string(rune('a'+n)),
				"https://example.com",
				"Page Title",
				n%3,
				[]string{"concurrent", "test", "word"},
			)
			idx.Add(page)
		}(i)
	}

	// 5 concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Search("concurrent")
		}()
	}

	wg.Wait()

	// All 10 pages should be indexed
	results := idx.Search("concurrent")
	if len(results) != 10 {
		t.Errorf("expected 10 results after concurrent adds, got %d", len(results))
	}
}

// TestCalculateScore verifies the scoring formula.
func TestCalculateScore(t *testing.T) {
	// Term appears once out of 10 total words, not in title, depth 0
	entry := IndexEntry{
		URL:        "https://example.com",
		TermCount:  1,
		TotalWords: 10,
		InTitle:    false,
		Depth:      0,
	}
	score := calculateScore(entry)
	if score <= 0 {
		t.Errorf("score should be positive, got %f", score)
	}

	// Formula: (freq×10) + 1000 - (depth×5)
	// depth=0, freq=1 → (1×10) + 1000 - (0×5) = 1010
	expected := float64(entry.TermCount*10) + 1000 - float64(entry.Depth*5)
	if score != expected {
		t.Errorf("score should be %f, got %f", expected, score)
	}

	// Depth penalty: deeper pages should score less
	deepEntry := entry
	deepEntry.Depth = 5
	deepScore := calculateScore(deepEntry)
	if deepScore >= score {
		t.Errorf("deeper page should score less: %f >= %f", deepScore, score)
	}
}
