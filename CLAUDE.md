cat > CLAUDE.md << 'EOF'
# Project Rules

## Language & Standards
- Language: Go (latest stable)
- Use ONLY Go standard library for core functionality
- Allowed external package: golang.org/x/net/html (for HTML parsing)
- Do NOT use Colly, Scrapy, or any crawling framework
- Do NOT use Beautiful Soup, Goquery, or similar high-level parsers

## Architecture
- Worker pool pattern with goroutines
- Buffered channels for URL queue (natural back pressure)
- sync.Mutex for visited set
- sync.RWMutex for inverted index (concurrent reads, exclusive writes)
- sync/atomic for metrics counters

## Code Style
- All exported functions must have comments
- Error handling: never ignore errors, always handle or log
- Use context.Context for cancellation support
- Keep functions short and focused (max ~50 lines)
- Use meaningful variable names (no single-letter except loop vars)

## Constraints
- No page should be crawled twice (visited set)
- Queue depth max: 1000 URLs
- Worker pool size: configurable, default 10
- HTTP timeout: 10 seconds per request
- Only crawl HTML pages (skip binary content)
- Respect depth limit strictly

## Testing
- Write unit tests for parser and index
- Use go test -race to verify no race conditions
EOF