package models

// CrawlTask represents a single URL to be crawled.
// It carries all the context needed: where it came from and how deep it is.
type CrawlTask struct {
	URL      string // The URL to crawl
	Origin   string // The original /index call's origin URL
	Depth    int    // Current depth (hops from origin)
	MaxDepth int    // Maximum allowed depth (k)
}

// PageData holds the extracted content of a single crawled page.
type PageData struct {
	URL    string   // The URL of this page
	Title  string   // Content of <title> tag
	Words  []string // All words found in the page body
	Links  []string // All href links found in the page
	Origin string   // Which /index call discovered this page
	Depth  int      // At what depth this page was found
}

// SearchResult is one entry in search results — the triple format.
type SearchResult struct {
	RelevantURL string  `json:"relevant_url"` // The page that matched the query
	OriginURL   string  `json:"origin_url"`   // The origin of the crawl that found it
	Depth       int     `json:"depth"`        // How many hops from origin
	Score       float64 `json:"score"`        // Relevancy score (higher = more relevant)
}

// CrawlHistoryEntry captures a snapshot of a completed or running crawl.
type CrawlHistoryEntry struct {
	Origin        string `json:"origin"`
	MaxDepth      int    `json:"max_depth"`
	URLsProcessed int64  `json:"urls_processed"`
	ErrorCount    int64  `json:"error_count"`
	StartedAt     int64  `json:"started_at"`  // Unix timestamp
	FinishedAt    int64  `json:"finished_at"` // Unix timestamp, 0 if still running
	Status        string `json:"status"`      // "running", "done", "stopped"
}

// SystemStatus holds real-time metrics for the dashboard.
type SystemStatus struct {
	URLsProcessed   int64  `json:"urls_processed"`    // How many pages we've crawled
	URLsQueued      int    `json:"urls_queued"`       // How many URLs waiting in queue
	QueueCapacity   int    `json:"queue_capacity"`    // Max queue size
	ActiveWorkers   int64  `json:"active_workers"`    // Currently working goroutines
	MaxWorkers      int    `json:"max_workers"`       // Configured max workers
	IsIndexing      bool   `json:"is_indexing"`       // Is a crawl currently running?
	IsPaused        bool   `json:"is_paused"`         // Is crawl currently paused?
	ErrorCount      int64  `json:"error_count"`       // Total errors encountered
	BackPressure    bool   `json:"back_pressure"`     // Is the queue near capacity?
	CurrentOrigin   string `json:"current_origin"`    // The URL being crawled from
	CurrentMaxDepth int    `json:"current_max_depth"` // The k value of current crawl
	MaxURLs         int64  `json:"max_urls"`          // Max URLs to visit (0 = unlimited)
	IndexSize       int    `json:"index_size"`        // Unique terms in the inverted index
	RateLimitMs     int64  `json:"rate_limit_ms"`     // Current rate limit in milliseconds
	StartedAt       int64  `json:"started_at"`        // Unix epoch when current crawl started
	FinishedAt      int64  `json:"finished_at"`       // Unix epoch when current crawl finished (0 if still running)
	CrawlStatus     string `json:"crawl_status"`      // "idle", "running", "paused", "stopped", "done"
}

// QueueItem represents a URL waiting in the crawl queue.
type QueueItem struct {
	URL   string `json:"url"`
	Depth int    `json:"depth"`
}

// Constants for CrawlStatus
const (
	CrawlStatusIdle    = "idle"
	CrawlStatusRunning = "running"
	CrawlStatusPaused  = "paused"
	CrawlStatusStopped = "stopped"
	CrawlStatusDone    = "done"
)
