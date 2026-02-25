package handlers

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipResponseWriter wraps http.ResponseWriter to gzip the response
type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.writer.Write(b)
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

// GzipMiddleware compresses responses for clients that accept gzip
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if client doesn't accept gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)
		gz.Reset(w)

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length changes after compression
		w.Header().Set("Vary", "Accept-Encoding")

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
		gz.Close()
	})
}
