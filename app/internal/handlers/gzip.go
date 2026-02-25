package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter wraps http.ResponseWriter to lazily apply gzip compression.
// It defers the decision to compress until the first Write, when Content-Type is known.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	gzipping    bool // true once we've decided to compress
	decided     bool // true once we've made the compress/skip decision
	wroteHeader bool // true once WriteHeader has been called
	statusCode  int
}

// WriteHeader captures the status code and defers the actual header write
func (g *gzipResponseWriter) WriteHeader(code int) {
	g.statusCode = code
	g.wroteHeader = true
	// Don't call underlying WriteHeader yet — wait until we decide on gzip
}

// Write sniffs Content-Type from the uncompressed data on first call,
// then decides whether to gzip based on the content type.
func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.decided {
		g.decided = true

		// If Content-Type wasn't set by the handler, sniff from uncompressed data
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(b))
		}

		ct := g.Header().Get("Content-Type")
		if isCompressible(ct) {
			g.gzipping = true
			g.Header().Set("Content-Encoding", "gzip")
			g.Header().Del("Content-Length") // Length changes after compression
			g.Header().Set("Vary", "Accept-Encoding")
			g.gz.Reset(g.ResponseWriter)
		}

		// Now flush the deferred status code
		if g.wroteHeader {
			g.ResponseWriter.WriteHeader(g.statusCode)
		}
	}

	if g.gzipping {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// gzip writer pool to reduce allocations
var gzipPool = sync.Pool{
	New: func() interface{} {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

// compressible content types worth gzipping
var compressibleTypes = map[string]bool{
	"text/html":              true,
	"text/css":               true,
	"application/javascript": true,
	"application/json":       true,
	"image/svg+xml":          true,
	"text/plain":             true,
}

// isCompressible checks if a content type should be gzip-compressed
func isCompressible(contentType string) bool {
	// Strip charset suffix: "text/css; charset=utf-8" → "text/css"
	ct := contentType
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}
	return compressibleTypes[ct]
}

// GzipMiddleware compresses responses for clients that accept gzip.
// Compression is applied lazily — the decision is deferred until the first
// Write call, when Content-Type is available for sniffing.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if client doesn't accept gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)

		grw := &gzipResponseWriter{
			ResponseWriter: w,
			gz:             gz,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(grw, r)

		// Close the gzip writer if we used it
		if grw.gzipping {
			gz.Close()
		}
	})
}
