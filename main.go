package main

import (
	"encoding/gob"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crawler/crawler"
	"crawler/index"
	"crawler/storage"
	"crawler/ui"
)

func init() {
	// Register IndexEntry with gob so it can be encoded/decoded
	// through an interface{} when saving/loading persistence files.
	gob.Register(map[string][]index.IndexEntry{})
}

const dataDir = "./crawl_data"

func main() {
	// ── Inverted Index ──────────────────────────────────────────────
	idx := index.NewInvertedIndex()

	// ── Persistence Manager ─────────────────────────────────────────
	pm := storage.NewPersistenceManager(dataDir)

	// Attempt to restore previous state on startup
	if pm.HasSavedState() {
		var snapshot map[string][]index.IndexEntry
		if ok, err := pm.LoadIndex(&snapshot); ok && err == nil {
			idx.Restore(snapshot)
			log.Printf("[Persistence] Restored index: %d terms loaded from disk", idx.Size())
		} else if err != nil {
			log.Printf("[Persistence] Could not load index: %v", err)
		}
	} else {
		log.Printf("[Persistence] No previous state found — starting fresh")
	}

	// ── File Store (data/storage/[letter].data) ──────────────────────
	fs := storage.NewFileStore()

	// ── Crawler ─────────────────────────────────────────────────────
	config := crawler.DefaultConfig()
	c := crawler.NewCrawler(config, idx, fs)

	// Restore visited URLs if available
	if pm.HasSavedState() {
		var visited map[string]bool
		if ok, err := pm.LoadVisited(&visited); ok && err == nil {
			c.RestoreVisited(visited)
			log.Printf("[Persistence] Restored %d visited URLs from disk", len(visited))
		}
	}

	// Auto-save index + visited URLs to disk every 10 seconds
	pm.StartAutoSave(10*time.Second, func() {
		snap := idx.Snapshot()
		if err := pm.SaveIndex(snap); err != nil {
			log.Printf("[Persistence] Save index error: %v", err)
		}
		visited := c.GetVisited()
		if err := pm.SaveVisited(visited); err != nil {
			log.Printf("[Persistence] Save visited error: %v", err)
		}
	})

	// ── Graceful shutdown ────────────────────────────────────────────
	// On SIGINT / SIGTERM: do a final save before exiting
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		log.Printf("[Persistence] Shutting down — performing final save...")
		pm.Stop()
		os.Exit(0)
	}()

	// ── HTTP Server ──────────────────────────────────────────────────
	handler := ui.NewHandler(c, idx, "ui/templates")

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := ":3600"
	log.Printf("Dashboard running at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
