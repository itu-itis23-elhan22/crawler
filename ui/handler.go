package ui

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"crawler/crawler"
	"crawler/index"
)

// Handler holds references to crawler and index for the API endpoints.
type Handler struct {
	crawler *crawler.Crawler
	idx     *index.InvertedIndex
	tmpl    *template.Template
}

// NewHandler creates a Handler and loads the HTML template.
func NewHandler(c *crawler.Crawler, idx *index.InvertedIndex, templateDir string) *Handler {
	tmplPath := filepath.Join(templateDir, "index.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Fatalf("Failed to load template %s: %v", tmplPath, err)
	}

	return &Handler{
		crawler: c,
		idx:     idx,
		tmpl:    tmpl,
	}
}

// RegisterRoutes sets up all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleDashboard)
	mux.HandleFunc("/index", h.handleIndex)
	mux.HandleFunc("/search", h.handleSearch)
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/history", h.handleHistory)
	mux.HandleFunc("/logs", h.handleLogs)
	mux.HandleFunc("/queue", h.handleQueue)
}

// handleDashboard serves the main HTML page.
func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	h.tmpl.Execute(w, nil)
}

// handleIndex starts, stops, pauses, or resumes a crawl.
//
//	POST /index        → start crawl
//	DELETE /index      → stop crawl
//	PATCH /index?action=pause  → pause crawl
//	PATCH /index?action=resume → resume crawl
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		h.crawler.StopCrawl()
		writeJSON(w, http.StatusOK, map[string]string{"status": "crawl stopped"})
		return

	case http.MethodPatch:
		action := r.URL.Query().Get("action")
		switch action {
		case "pause":
			h.crawler.PauseCrawl()
			writeJSON(w, http.StatusOK, map[string]string{"status": "crawl paused"})
		case "resume":
			h.crawler.ResumeCrawl()
			writeJSON(w, http.StatusOK, map[string]string{"status": "crawl resumed"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "action must be 'pause' or 'resume'",
			})
		}
		return

	case http.MethodPost:
		h.handleStartCrawl(w, r)
		return

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "use POST, DELETE, or PATCH method",
		})
	}
}

// handleStartCrawl processes POST /index to start a new crawl.
func (h *Handler) handleStartCrawl(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Origin      string `json:"origin"`
		Depth       int    `json:"depth"`
		Workers     int    `json:"workers"`       // optional, default 10
		RateLimitMs int64  `json:"rate_limit_ms"` // optional, default 200
		QueueSize   int    `json:"queue_size"`    // optional, default 1000
		MaxURLs     int64  `json:"max_urls"`      // optional, 0 = unlimited
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON: " + err.Error(),
		})
		return
	}

	// Validate required inputs
	if req.Origin == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "origin URL is required",
		})
		return
	}
	if req.Depth < 0 || req.Depth > 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "depth must be between 0 and 10",
		})
		return
	}
	if req.Workers < 0 || req.Workers > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "workers must be between 1 and 50",
		})
		return
	}
	if req.RateLimitMs < 0 || req.RateLimitMs > 10000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "rate_limit_ms must be between 0 and 10000",
		})
		return
	}
	if req.MaxURLs < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "max_urls must be >= 0",
		})
		return
	}

	if err := h.crawler.StartCrawl(req.Origin, req.Depth, req.Workers, req.RateLimitMs, req.QueueSize, req.MaxURLs); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "crawl started",
		"origin": req.Origin,
		"depth":  strconv.Itoa(req.Depth),
	})
}

// handleSearch searches the index. GET /search?query=keyword&limit=50
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "use GET method",
		})
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "query parameter is required",
		})
		return
	}

	// Parse optional limit param (default 50, max 200)
	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	results := h.idx.Search(query)

	// Apply limit
	total := len(results)
	if limit < total {
		results = results[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":       query,
		"count":       len(results),
		"total_found": total,
		"results":     results,
	})
}

// handleStatus returns current system metrics. GET /status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := h.crawler.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

// handleHistory returns the list of past crawl jobs. GET /history
func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	history := h.crawler.GetHistory()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"history": history,
		"count":   len(history),
	})
}

// handleLogs returns the crawler log buffer.
// GET /logs          → JSON array
// GET /logs?format=text → plain text for download
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "use GET method",
		})
		return
	}

	logs := h.crawler.GetLogs()

	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"crawler_logs.txt\"")
		w.WriteHeader(http.StatusOK)
		for _, entry := range logs {
			w.Write([]byte(entry + "\n"))
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"count": len(logs),
	})
}

// handleQueue returns the current URL queue contents.
// GET /queue          → JSON array
// GET /queue?format=text → plain text for download
func (h *Handler) handleQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "use GET method",
		})
		return
	}

	items := h.crawler.GetCurrentQueue()

	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"crawler_queue.txt\"")
		w.WriteHeader(http.StatusOK)
		var sb strings.Builder
		for _, item := range items {
			sb.WriteString(item.URL)
			sb.WriteString(" (depth: ")
			sb.WriteString(strconv.Itoa(item.Depth))
			sb.WriteString(")\n")
		}
		w.Write([]byte(sb.String()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"queue": items,
		"count": len(items),
	})
}

// writeJSON is a helper to send JSON responses.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
