package crawler

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"crawler/index"
	"crawler/models"
	"crawler/storage"
)

// Config holds all configurable parameters for the crawler.
type Config struct {
	MaxWorkers   int           // Number of concurrent goroutines
	QueueSize    int           // Maximum URLs in the queue (back pressure)
	FetchTimeout time.Duration // HTTP request timeout
	RateLimit    time.Duration // Minimum delay between requests per worker
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() Config {
	return Config{
		MaxWorkers:   10,
		QueueSize:    1000,
		FetchTimeout: 10 * time.Second,
		RateLimit:    200 * time.Millisecond,
	}
}

// maxLogEntries is the size of the per-crawl log ring buffer.
const maxLogEntries = 500

// Crawler orchestrates the entire crawling process.
type Crawler struct {
	config    Config
	fetcher   *Fetcher
	idx       *index.InvertedIndex
	fileStore *storage.FileStore

	// Visited set: tracks which URLs we've already crawled
	visitedMu sync.Mutex
	visited   map[string]bool

	// Metrics (atomic for lock-free thread safety)
	urlsProcessed int64
	activeWorkers int64
	errorCount    int64

	// State
	queue      chan models.CrawlTask
	isIndexing int32 // 0 = not indexing, 1 = indexing (atomic)
	isPaused   int32 // 0 = not paused, 1 = paused (atomic)
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup

	// Pause/resume coordination
	pauseCh chan struct{} // closed to signal resume
	pauseMu sync.Mutex    // guards pauseCh replacement

	// Limit on total URLs to visit (0 = unlimited)
	maxURLs int64

	// Per-crawl log buffer (ring buffer, capped at maxLogEntries)
	logMu  sync.Mutex
	logBuf []string

	// Current crawl info
	currentOrigin   string
	currentMaxDepth int
	crawlStartedAt  int64 // Unix timestamp
	crawlFinishedAt int64 // Unix timestamp (0 if still running)
	crawlStatus     string

	// History of past crawls (most recent last)
	historyMu sync.Mutex
	history   []models.CrawlHistoryEntry
}

// NewCrawler creates a new Crawler with the given config, shared index, and file store.
func NewCrawler(config Config, idx *index.InvertedIndex, fs *storage.FileStore) *Crawler {
	pauseCh := make(chan struct{})
	close(pauseCh) // Start unpaused: reads on a closed channel return immediately
	return &Crawler{
		config:      config,
		fetcher:     NewFetcher(config.FetchTimeout),
		idx:         idx,
		fileStore:   fs,
		visited:     make(map[string]bool),
		queue:       make(chan models.CrawlTask, config.QueueSize),
		crawlStatus: models.CrawlStatusIdle,
		pauseCh:     pauseCh,
	}
}

// StartCrawl begins crawling from the origin URL up to maxDepth.
// It accepts optional overrides for workers, rate limit, queue size, and max URLs.
// It launches worker goroutines and returns immediately (non-blocking).
func (c *Crawler) StartCrawl(origin string, maxDepth int, workers int, rateLimitMs int64, queueSize int, maxURLs int64) error {
	// Don't start if already crawling
	if !atomic.CompareAndSwapInt32(&c.isIndexing, 0, 1) {
		return fmt.Errorf("crawl already in progress")
	}

	// Validate the origin URL
	_, err := url.Parse(origin)
	if err != nil {
		atomic.StoreInt32(&c.isIndexing, 0)
		return fmt.Errorf("invalid origin URL: %w", err)
	}

	// Apply optional overrides
	if workers > 0 {
		c.config.MaxWorkers = workers
	}
	if rateLimitMs > 0 {
		c.config.RateLimit = time.Duration(rateLimitMs) * time.Millisecond
	}
	if queueSize > 0 && queueSize != cap(c.queue) {
		// Recreate queue with new capacity
		c.queue = make(chan models.CrawlTask, queueSize)
		c.config.QueueSize = queueSize
	}

	// Store max URLs limit (0 = unlimited)
	atomic.StoreInt64(&c.maxURLs, maxURLs)

	// Store current crawl info
	c.currentOrigin = origin
	c.currentMaxDepth = maxDepth
	now := time.Now().Unix()
	c.crawlStartedAt = now
	c.crawlFinishedAt = 0
	c.crawlStatus = models.CrawlStatusRunning

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFunc = cancel

	// Reset metrics for new crawl
	atomic.StoreInt64(&c.urlsProcessed, 0)
	atomic.StoreInt64(&c.errorCount, 0)

	// Clear visited set for fresh crawl
	c.visitedMu.Lock()
	c.visited = make(map[string]bool)
	c.visitedMu.Unlock()

	// Reset log buffer and ensure crawler starts unpaused
	c.logMu.Lock()
	c.logBuf = c.logBuf[:0]
	c.logMu.Unlock()

	// Ensure unpaused state for new crawl
	atomic.StoreInt32(&c.isPaused, 0)
	c.pauseMu.Lock()
	// Replace pauseCh with a fresh closed channel (means "not paused")
	newPauseCh := make(chan struct{})
	close(newPauseCh)
	c.pauseCh = newPauseCh
	c.pauseMu.Unlock()

	// Seed the queue with the origin URL
	c.queue <- models.CrawlTask{
		URL:      origin,
		Origin:   origin,
		Depth:    0,
		MaxDepth: maxDepth,
	}

	// Launch worker goroutines
	for i := 0; i < c.config.MaxWorkers; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i)
	}

	// Launch a goroutine to wait for all workers to finish
	go func() {
		c.wg.Wait()
		finished := time.Now().Unix()
		c.crawlFinishedAt = finished
		// Only mark done if not manually stopped
		if c.crawlStatus == models.CrawlStatusRunning {
			c.crawlStatus = models.CrawlStatusDone
		}
		atomic.StoreInt32(&c.isIndexing, 0)

		// Save to history
		c.historyMu.Lock()
		c.history = append(c.history, models.CrawlHistoryEntry{
			Origin:        origin,
			MaxDepth:      maxDepth,
			URLsProcessed: atomic.LoadInt64(&c.urlsProcessed),
			ErrorCount:    atomic.LoadInt64(&c.errorCount),
			StartedAt:     now,
			FinishedAt:    finished,
			Status:        c.crawlStatus,
		})
		c.historyMu.Unlock()

		log.Printf("Crawl complete. Processed: %d, Errors: %d",
			atomic.LoadInt64(&c.urlsProcessed),
			atomic.LoadInt64(&c.errorCount))
	}()

	log.Printf("Started crawling %s with depth %d using %d workers (rate=%dms, queue=%d)",
		origin, maxDepth, c.config.MaxWorkers,
		c.config.RateLimit.Milliseconds(), cap(c.queue))
	return nil
}

// StopCrawl cancels the current crawl operation.
func (c *Crawler) StopCrawl() {
	if c.cancelFunc != nil {
		// If paused, resume first so workers can notice ctx cancellation
		atomic.StoreInt32(&c.isPaused, 0)
		c.pauseMu.Lock()
		select {
		case <-c.pauseCh:
			// already unblocked
		default:
			close(c.pauseCh)
		}
		c.pauseMu.Unlock()

		c.crawlStatus = models.CrawlStatusStopped
		c.cancelFunc()
	}
}

// PauseCrawl suspends worker activity without cancelling the context.
func (c *Crawler) PauseCrawl() {
	if atomic.LoadInt32(&c.isIndexing) == 0 {
		return
	}
	if atomic.CompareAndSwapInt32(&c.isPaused, 0, 1) {
		c.pauseMu.Lock()
		// Replace with a fresh open channel — workers will block on it
		c.pauseCh = make(chan struct{})
		c.pauseMu.Unlock()
		c.crawlStatus = models.CrawlStatusPaused
		c.appendLog("Crawl paused")
	}
}

// ResumeCrawl unblocks workers that are waiting on the pause channel.
func (c *Crawler) ResumeCrawl() {
	if atomic.CompareAndSwapInt32(&c.isPaused, 1, 0) {
		c.pauseMu.Lock()
		select {
		case <-c.pauseCh:
			// Already closed somehow
		default:
			close(c.pauseCh)
		}
		c.pauseMu.Unlock()
		c.crawlStatus = models.CrawlStatusRunning
		c.appendLog("Crawl resumed")
	}
}

// appendLog adds a timestamped message to the in-memory log buffer.
// When the buffer is full the oldest entry is dropped.
func (c *Crawler) appendLog(msg string) {
	entry := fmt.Sprintf("%s - %s", time.Now().Format("2006-01-02 15:04:05"), msg)
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if len(c.logBuf) >= maxLogEntries {
		// Drop the oldest entry (ring-buffer behaviour)
		c.logBuf = c.logBuf[1:]
	}
	c.logBuf = append(c.logBuf, entry)
}

// worker is a goroutine that pulls tasks from the queue and processes them.
func (c *Crawler) worker(ctx context.Context, id int) {
	defer c.wg.Done()

	for {
		// Wait while paused (or return immediately if not paused)
		c.pauseMu.Lock()
		pauseCh := c.pauseCh
		c.pauseMu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-pauseCh:
			// Not paused (or just resumed) — continue
		}

		select {
		case <-ctx.Done():
			return

		case task, ok := <-c.queue:
			if !ok {
				return
			}

			atomic.AddInt64(&c.activeWorkers, 1)
			c.processTask(ctx, task)
			atomic.AddInt64(&c.activeWorkers, -1)

			// Rate limiting
			time.Sleep(c.config.RateLimit)

		default:
			if atomic.LoadInt64(&c.activeWorkers) == 0 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// processTask handles a single crawl task: fetch, parse, index, enqueue new links.
func (c *Crawler) processTask(ctx context.Context, task models.CrawlTask) {
	// Check MaxURLs limit before processing
	limit := atomic.LoadInt64(&c.maxURLs)
	if limit > 0 && atomic.LoadInt64(&c.urlsProcessed) >= limit {
		return
	}

	if !c.markVisited(task.URL) {
		return
	}

	c.appendLog(fmt.Sprintf("Crawling %s at depth %d", task.URL, task.Depth))

	html, err := c.fetcher.Fetch(ctx, task.URL)
	if err != nil {
		atomic.AddInt64(&c.errorCount, 1)
		msg := fmt.Sprintf("Error fetching %s: %v", task.URL, err)
		log.Printf("[Worker] %s", msg)
		c.appendLog(msg)
		return
	}

	c.appendLog(fmt.Sprintf("Successfully accessed %s", task.URL))

	pageData := ParseHTML(html, task.URL, task.Origin, task.Depth)
	c.idx.Add(pageData)
	atomic.AddInt64(&c.urlsProcessed, 1)

	// Write word entries to data/storage/[letter].data files
	if err := c.fileStore.WriteWords(pageData.Words, task.URL, task.Origin, task.Depth); err != nil {
		log.Printf("[FileStore] Write error for %s: %v", task.URL, err)
	}

	c.appendLog(fmt.Sprintf("Stored %d unique words from %s", len(pageData.Words), task.URL))

	if task.Depth < task.MaxDepth {
		newLinks := c.enqueueLinks(pageData.Links, task.Origin, task.Depth+1, task.MaxDepth, ctx)
		c.appendLog(fmt.Sprintf("Found %d new URLs at %s", newLinks, task.URL))
	}

	log.Printf("[Worker] Indexed %s (depth=%d, links=%d, words=%d)",
		task.URL, task.Depth, len(pageData.Links), len(pageData.Words))
}

// markVisited atomically checks if a URL was visited and marks it if not.
func (c *Crawler) markVisited(rawURL string) bool {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()

	if c.visited[rawURL] {
		return false
	}
	c.visited[rawURL] = true
	return true
}

// enqueueLinks adds discovered links to the crawl queue.
// Non-blocking: if queue is full, skips the link (back pressure).
// Returns the count of links successfully enqueued.
func (c *Crawler) enqueueLinks(links []string, origin string, depth int, maxDepth int, ctx context.Context) int {
	enqueued := 0
	for _, link := range links {
		select {
		case <-ctx.Done():
			return enqueued
		default:
		}

		c.visitedMu.Lock()
		alreadyVisited := c.visited[link]
		c.visitedMu.Unlock()

		if alreadyVisited {
			continue
		}

		task := models.CrawlTask{
			URL:      link,
			Origin:   origin,
			Depth:    depth,
			MaxDepth: maxDepth,
		}

		select {
		case c.queue <- task:
			enqueued++
		default:
			msg := fmt.Sprintf("[BackPressure] Queue full (%d/%d), skipping: %s",
				len(c.queue), cap(c.queue), link)
			log.Print(msg)
			c.appendLog(msg)
		}
	}
	return enqueued
}

// GetStatus returns the current system status for the dashboard.
func (c *Crawler) GetStatus() models.SystemStatus {
	queueLen := len(c.queue)
	queueCap := cap(c.queue)

	return models.SystemStatus{
		URLsProcessed:   atomic.LoadInt64(&c.urlsProcessed),
		URLsQueued:      queueLen,
		QueueCapacity:   queueCap,
		ActiveWorkers:   atomic.LoadInt64(&c.activeWorkers),
		MaxWorkers:      c.config.MaxWorkers,
		IsIndexing:      atomic.LoadInt32(&c.isIndexing) == 1,
		IsPaused:        atomic.LoadInt32(&c.isPaused) == 1,
		ErrorCount:      atomic.LoadInt64(&c.errorCount),
		BackPressure:    queueLen > (queueCap * 80 / 100),
		CurrentOrigin:   c.currentOrigin,
		CurrentMaxDepth: c.currentMaxDepth,
		MaxURLs:         atomic.LoadInt64(&c.maxURLs),
		IndexSize:       c.idx.Size(),
		RateLimitMs:     c.config.RateLimit.Milliseconds(),
		StartedAt:       c.crawlStartedAt,
		CrawlStatus:     c.crawlStatus,
	}
}

// GetLogs returns a copy of the current log buffer.
func (c *Crawler) GetLogs() []string {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	result := make([]string, len(c.logBuf))
	copy(result, c.logBuf)
	return result
}

// GetCurrentQueue returns a snapshot of URLs currently in the queue.
func (c *Crawler) GetCurrentQueue() []models.QueueItem {
	// Drain into a temporary slice without blocking the queue
	snapshot := make([]models.CrawlTask, 0, len(c.queue))
	remaining := len(c.queue)
	for i := 0; i < remaining; i++ {
		select {
		case task := <-c.queue:
			snapshot = append(snapshot, task)
		default:
		}
	}
	// Put tasks back (best-effort, may drop if queue filled in the meantime)
	for _, task := range snapshot {
		select {
		case c.queue <- task:
		default:
		}
	}
	items := make([]models.QueueItem, len(snapshot))
	for i, t := range snapshot {
		items[i] = models.QueueItem{URL: t.URL, Depth: t.Depth}
	}
	return items
}

// GetHistory returns the list of past crawl jobs (excludes currently running one).
func (c *Crawler) GetHistory() []models.CrawlHistoryEntry {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	result := make([]models.CrawlHistoryEntry, len(c.history))
	copy(result, c.history)
	return result
}

// GetVisitedCount returns how many unique URLs have been visited.
func (c *Crawler) GetVisitedCount() int {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	return len(c.visited)
}

// GetVisited returns a snapshot of the visited URL set for persistence.
func (c *Crawler) GetVisited() map[string]bool {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	snap := make(map[string]bool, len(c.visited))
	for k, v := range c.visited {
		snap[k] = v
	}
	return snap
}

// RestoreVisited loads a previously persisted visited set.
// Call this once before starting the server to resume a previous crawl.
func (c *Crawler) RestoreVisited(visited map[string]bool) {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	c.visited = visited
}
