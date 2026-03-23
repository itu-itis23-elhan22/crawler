# Product Requirements Document
## Web Crawler & Search Engine

**Repository:** https://github.com/itu-itis23-elhan22/crawler

---

## 1. Overview

Build a concurrent web crawler and real-time search engine in Go that runs on a single machine. The system must crawl web pages starting from a given URL up to a specified depth, build a keyword-based inverted index, and allow search queries to be answered while indexing is still active. The design must demonstrate architectural clarity, thread safety, and controlled back pressure.

---

## 2. Functional Requirements

### 2.1 Indexer — `POST /index`

**Inputs:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `origin` | string | Yes | Starting URL for the crawl |
| `depth` | int | Yes | Maximum hops (k) from origin (0–10) |
| `workers` | int | No | Worker goroutine count (default: 10, max: 50) |
| `rate_limit_ms` | int64 | No | Milliseconds between requests per worker (default: 200) |
| `queue_size` | int | No | URL queue buffer capacity (default: 1000) |
| `max_urls` | int64 | No | Maximum pages to index, 0 = unlimited |

**Behavior:**
- Crawl begins at `origin` (depth 0). Each page's links are discovered and placed in the queue at `depth + 1`.
- A URL is never crawled twice — a visited set (hash map protected by `sync.Mutex`) tracks all seen URLs.
- Workers stop enqueuing links when `depth == max_depth`.
- If `max_urls > 0`, workers stop indexing once that count is reached.
- Fetcher only processes `text/html` content. Binary files, PDFs, and images are skipped.
- HTTP timeout is 10 seconds per request. Pages larger than 10 MB are skipped.
- Back pressure: when the queue channel is full, new URLs are logged and dropped (non-blocking send).

**Controls:**
- `DELETE /index` — cancel the current crawl immediately
- `PATCH /index?action=pause` — suspend workers without cancelling the context
- `PATCH /index?action=resume` — unblock suspended workers

### 2.2 Searcher — `GET /search`

**Inputs:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search term(s), space-separated |
| `limit` | int | No | Maximum results to return (default: 50, max: 200) |

**Returns:** A JSON array of triples sorted by relevancy score:
```json
[
  {
    "relevant_url": "https://go.dev/doc/",
    "origin_url":   "https://go.dev/",
    "depth":        1,
    "score":        4.762
  }
]
```

**Relevancy Formula:**
```
score = TF x title_boost x depth_factor

TF           = term_count / total_words_on_page
title_boost  = 2.0  if term appears in <title>, else 1.0
depth_factor = 1.0 / (1 + depth x 0.1)
```

**Live Search:** Search runs concurrently with indexing. The inverted index is protected by `sync.RWMutex`, so multiple search queries can run simultaneously alongside ongoing writes.

### 2.3 System Visibility

**Dashboard (`GET /`):** Single-page web UI at `http://localhost:8080` with:
- Start / Stop / Pause / Resume crawl controls
- Origin URL, depth, max URLs, and advanced options (workers, rate limit, queue capacity)
- Real-time metrics: URLs processed, queue depth, active workers, index size, error count
- Queue fill progress bar with back-pressure indicator
- Live log viewer (updated every 2 seconds) showing per-page crawl events
- Search panel with result limit selector
- Crawl history with timestamps and status badges

**Additional API Endpoints:**

| Endpoint | Description |
|----------|-------------|
| `GET /status` | JSON: all real-time metrics |
| `GET /history` | JSON: list of completed/active crawl sessions |
| `GET /logs` | JSON: per-crawl log buffer (last 500 entries) |
| `GET /logs?format=text` | Plain-text log download |
| `GET /queue` | JSON: current URL queue snapshot |
| `GET /queue?format=text` | Plain-text queue download |

---

## 3. Non-Functional Requirements

### 3.1 Concurrency & Thread Safety

| Resource | Mechanism | Guarantee |
|----------|-----------|-----------|
| Visited set | `sync.Mutex` | No URL is crawled twice |
| Inverted index | `sync.RWMutex` | Concurrent reads, exclusive writes |
| URL queue | Buffered `chan CrawlTask` | Thread-safe; drops overflow non-blocking |
| Metrics | `sync/atomic` | Lock-free counter increments |
| Pause state | atomic + `chan struct{}` | All workers block/unblock atomically |
| Log buffer | `sync.Mutex` | Ring-buffer of last 500 entries |

Must pass `go test -race ./...` with no races detected.

### 3.2 Back Pressure

Four mechanisms work together to self-regulate load:
1. **Queue depth limit** — channel buffer acts as a hard ceiling; overflow is dropped
2. **Worker pool size** — caps parallel HTTP requests
3. **Rate limit** — configurable per-worker sleep between requests
4. **Max URLs cap** — optional hard limit on total pages per session

### 3.3 Persistence (Bonus)

- Index and visited set are serialized to `./crawl_data/` using `encoding/gob`
- Auto-save runs every 10 seconds in a background goroutine
- Atomic write: data is written to a `.tmp` file first, then renamed
- On startup, the system restores previous state automatically
- On SIGINT/SIGTERM, a final save is performed before exit

---

## 4. Technical Constraints

- Language: Go (latest stable)
- Only allowed external package: `golang.org/x/net/html`
- Do NOT use: Colly, Scrapy, Goquery, or any crawling framework
- HTTP client: `net/http` standard library only
- No external databases, message queues, or caches
- Runs on localhost; single-machine assumption

---

## 5. Project Structure (as built)

```
crawler_submission/
├── main.go                    # Server setup, persistence, graceful shutdown
├── crawler/
│   ├── crawler.go             # Worker pool, pause/resume, log buffer, max URLs
│   ├── fetcher.go             # HTTP client: timeout, size limit, content-type check
│   ├── parser.go              # Regex-based link + word extraction
│   └── parser_test.go         # Unit tests: extractTitle, extractLinks, extractWords
├── index/
│   ├── index.go               # Inverted index: Add, Search, Size, Snapshot, Restore
│   └── index_test.go          # Unit tests: scoring, concurrency, snapshot round-trip
├── models/
│   └── models.go              # Shared types: CrawlTask, SearchResult, SystemStatus
├── storage/
│   └── persistence.go         # Gob save/load, auto-save, atomic file writes
├── ui/
│   ├── handler.go             # HTTP handlers for all API endpoints
│   └── templates/
│       └── index.html         # Dashboard: controls, metrics, live logs, search
├── go.mod
├── readme.md
├── product_prd.md
└── recommendation.md
```

---

## 6. Success Criteria

| Category | Weight | Criteria |
|----------|--------|----------|
| Functionality | 40% | Crawler indexes without duplicates; search returns ranked triples; concurrent search works during indexing |
| Architecture | 40% | Worker pool pattern; back pressure via queue; thread-safe data; no race conditions |
| AI Stewardship | 20% | Developer can explain all design decisions; code is clean and testable |

---

## 7. Design Decisions & Rationale

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Queue | Buffered `chan CrawlTask` | Built-in thread safety; natural back pressure when full |
| Visited Set | `map[string]bool` + `sync.Mutex` | O(1) lookup; simple and correct |
| Inverted Index | `map[string][]IndexEntry` + `sync.RWMutex` | Multiple concurrent searches + single writer |
| Pause | `chan struct{}` (open/closed) | All workers block on a read; closing unblocks all simultaneously |
| Persistence | `encoding/gob` + atomic rename | No external DB; fast binary encoding; crash-safe |
| HTML Parsing | `regexp` | Avoids external dependency; sufficient for well-formed HTML |
| Scoring | TF + title boost + depth penalty | Simple, interpretable; effective for keyword matching |
