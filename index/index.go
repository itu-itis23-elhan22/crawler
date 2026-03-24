package index

import (
	"sort"
	"strings"
	"sync"

	"crawler/models"
)

// IndexEntry represents one page's association with a term.
type IndexEntry struct {
	URL        string
	Origin     string
	Depth      int
	TermCount  int  // How many times the term appears on this page
	InTitle    bool // Whether the term appears in the page title
	TotalWords int  // Total word count of the page
}

// InvertedIndex is a thread-safe mapping from terms to pages.
// It uses sync.RWMutex so multiple goroutines can search (read)
// simultaneously while only one can update (write) at a time.
type InvertedIndex struct {
	mu    sync.RWMutex
	index map[string][]IndexEntry
}

// NewInvertedIndex creates an empty inverted index.
func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		index: make(map[string][]IndexEntry),
	}
}

// Add indexes a page's content into the inverted index.
// Called by crawler workers after parsing a page.
func (idx *InvertedIndex) Add(page models.PageData) {
	// Step 1: Count frequency of each word on this page
	wordCounts := make(map[string]int)
	for _, word := range page.Words {
		wordCounts[word]++
	}

	// Step 2: Check which words appear in the title
	titleLower := strings.ToLower(page.Title)
	titleWords := make(map[string]bool)
	for _, word := range strings.Fields(titleLower) {
		titleWords[word] = true
	}

	totalWords := len(page.Words)

	// Step 3: Lock for writing — no one else can read or write
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Step 4: For each unique word, add an entry to the index
	for word, count := range wordCounts {
		entry := IndexEntry{
			URL:        page.URL,
			Origin:     page.Origin,
			Depth:      page.Depth,
			TermCount:  count,
			InTitle:    titleWords[word],
			TotalWords: totalWords,
		}
		idx.index[word] = append(idx.index[word], entry)
	}
}

// Search finds all pages relevant to the query and returns ranked results.
// Uses RLock so multiple searches can run concurrently.
func (idx *InvertedIndex) Search(query string) []models.SearchResult {
	// Normalize query: lowercase and split into terms
	query = strings.ToLower(strings.TrimSpace(query))
	terms := strings.Fields(query)

	if len(terms) == 0 {
		return nil
	}

	// Read lock — multiple goroutines can search at the same time
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Collect scores for each URL across all query terms
	urlScores := make(map[string]*scoredResult)

	for _, term := range terms {
		entries, exists := idx.index[term]
		if !exists {
			continue
		}

		for _, entry := range entries {
			key := entry.URL
			if _, found := urlScores[key]; !found {
				urlScores[key] = &scoredResult{
					url:    entry.URL,
					origin: entry.Origin,
					depth:  entry.Depth,
					score:  0,
				}
			}

			// Calculate relevancy score for this term-page pair
			score := calculateScore(entry)
			urlScores[key].score += score
		}
	}

	// Convert map to sorted slice
	var results []models.SearchResult
	for _, sr := range urlScores {
		results = append(results, models.SearchResult{
			RelevantURL: sr.url,
			OriginURL:   sr.origin,
			Depth:       sr.depth,
			Score:       sr.score,
		})
	}

	// Sort by score descending (most relevant first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// Size returns the number of unique terms in the index.
func (idx *InvertedIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.index)
}

// Snapshot returns a deep copy of the index map for persistence.
// Safe to call concurrently with Add and Search.
func (idx *InvertedIndex) Snapshot() map[string][]IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	copy := make(map[string][]IndexEntry, len(idx.index))
	for k, v := range idx.index {
		copy[k] = append([]IndexEntry(nil), v...)
	}
	return copy
}

// Restore loads a previously saved snapshot into the index.
// Replaces the current index content. Safe to call before serving traffic.
func (idx *InvertedIndex) Restore(snapshot map[string][]IndexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.index = snapshot
}

// scoredResult is an internal struct for accumulating scores.
type scoredResult struct {
	url    string
	origin string
	depth  int
	score  float64
}

// calculateScore computes a relevancy score for a term-page pair.
// Formula: (frequency × 10) + 1000 (exact match bonus) - (depth × 5)
func calculateScore(entry IndexEntry) float64 {
	score := float64(entry.TermCount*10) + 1000 - float64(entry.Depth*5)
	if score < 0 {
		score = 0
	}
	return score
}
