package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unicode"
)

const storageDir = "data/storage"

// FileStore writes indexed word entries to per-letter text files under data/storage/.
// Each line format: word url origin depth frequency
// e.g.:  program https://go.dev/doc/ https://go.dev/ 1 5
type FileStore struct {
	mu  sync.Mutex
	dir string
}

// NewFileStore creates the data/storage directory and returns a FileStore.
func NewFileStore() *FileStore {
	os.MkdirAll(storageDir, 0755)
	return &FileStore{dir: storageDir}
}

// WriteWords counts word frequencies from the words slice and appends entries
// to the corresponding per-letter file (e.g. "p.data" for words starting with 'p').
func (fs *FileStore) WriteWords(words []string, url, origin string, depth int) error {
	// Count frequency of each word
	counts := make(map[string]int)
	for _, w := range words {
		if w != "" {
			counts[w]++
		}
	}

	// Group lines by first letter
	byLetter := make(map[string][]string)
	for word, freq := range counts {
		runes := []rune(word)
		if len(runes) == 0 {
			continue
		}
		first := unicode.ToLower(runes[0])
		if !unicode.IsLetter(first) {
			continue
		}
		letter := string(first)
		line := fmt.Sprintf("%s %s %s %d %d\n", word, url, origin, depth, freq)
		byLetter[letter] = append(byLetter[letter], line)
	}

	if len(byLetter) == 0 {
		return nil
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	for letter, lines := range byLetter {
		filename := filepath.Join(fs.dir, letter+".data")
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("filestore open %s: %w", filename, err)
		}
		for _, line := range lines {
			if _, werr := f.WriteString(line); werr != nil {
				f.Close()
				return fmt.Errorf("filestore write %s: %w", filename, werr)
			}
		}
		f.Close()
	}
	return nil
}
