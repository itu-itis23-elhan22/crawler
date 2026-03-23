// Package storage provides persistence for the crawler's state.
// It saves the inverted index and visited URLs to disk using encoding/gob,
// so crawls can be resumed after process interruption (the bonus feature).
package storage

import (
	"encoding/gob"
	"log"
	"os"
	"sync"
	"time"
)

// PersistenceManager handles automatic saving of crawl state to disk.
type PersistenceManager struct {
	dataDir     string
	indexFile   string
	visitedFile string
	mu          sync.Mutex
	stopCh      chan struct{}
}

// NewPersistenceManager creates a manager that saves state to dataDir.
func NewPersistenceManager(dataDir string) *PersistenceManager {
	os.MkdirAll(dataDir, 0755)
	return &PersistenceManager{
		dataDir:     dataDir,
		indexFile:   dataDir + "/index.gob",
		visitedFile: dataDir + "/visited.gob",
		stopCh:      make(chan struct{}),
	}
}

// StartAutoSave begins a background goroutine that saves state every interval.
func (p *PersistenceManager) StartAutoSave(interval time.Duration, saveFn func()) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				saveFn()
			case <-p.stopCh:
				// Final save on shutdown
				saveFn()
				return
			}
		}
	}()
	log.Printf("[Persistence] Auto-save started (every %v) → %s", interval, p.dataDir)
}

// Stop signals the auto-save goroutine to do a final save and exit.
func (p *PersistenceManager) Stop() {
	close(p.stopCh)
}

// SaveIndex writes the inverted index map to disk.
func (p *PersistenceManager) SaveIndex(data interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return gobSave(p.indexFile, data)
}

// LoadIndex reads the inverted index map from disk.
// Returns nil and no error if the file doesn't exist (fresh start).
func (p *PersistenceManager) LoadIndex(out interface{}) (bool, error) {
	return gobLoad(p.indexFile, out)
}

// SaveVisited writes the visited URL set to disk.
func (p *PersistenceManager) SaveVisited(visited map[string]bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return gobSave(p.visitedFile, visited)
}

// LoadVisited reads the visited URL set from disk.
// Returns (false, nil) if no file exists (fresh start).
func (p *PersistenceManager) LoadVisited(out *map[string]bool) (bool, error) {
	return gobLoad(p.visitedFile, out)
}

// HasSavedState returns true if persisted data files exist.
func (p *PersistenceManager) HasSavedState() bool {
	_, err1 := os.Stat(p.indexFile)
	_, err2 := os.Stat(p.visitedFile)
	return err1 == nil && err2 == nil
}

// ClearSavedState deletes the persisted state files.
func (p *PersistenceManager) ClearSavedState() {
	os.Remove(p.indexFile)
	os.Remove(p.visitedFile)
	log.Printf("[Persistence] Cleared saved state from %s", p.dataDir)
}

// gobSave encodes data to a gob file atomically (write to temp, rename).
func gobSave(path string, data interface{}) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}

// gobLoad decodes a gob file into out. Returns (false, nil) if file missing.
func gobLoad(path string, out interface{}) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	if err := gob.NewDecoder(f).Decode(out); err != nil {
		return false, err
	}
	return true, nil
}
