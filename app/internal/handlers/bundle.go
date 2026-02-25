package handlers

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Bundle holds a pre-built concatenated asset bundle served from memory
type Bundle struct {
	Content     []byte
	ContentType string
	ETag        string
}

// Bundles maps URL paths to pre-built asset bundles
var Bundles map[string]*Bundle

// Public CSS bundle — order matches main.css @import cascade
var publicCSSFiles = []string{
	"web/static/css/base.css",
	"web/static/css/cards.css",
	"web/static/css/uptime.css",
	"web/static/css/resources.css",
	"web/static/css/day-detail.css",
	"web/static/css/admin.css",
	"web/static/css/dialogs.css",
	"web/static/css/banners.css",
	"web/static/css/matrix.css",
	"web/static/css/mobile.css",
	"web/static/css/blocks.css",
}

// Admin CSS bundle — lazy-loaded for authenticated users only
var adminCSSFiles = []string{
	"web/static/css/admin-tabs.css",
	"web/static/css/settings.css",
	"web/static/css/logs.css",
}

// Public JS bundle — order matches index.html script loading order
var publicJSFiles = []string{
	"web/static/js/utils.js",
	"web/static/js/core.js",
	"web/static/js/resources.js",
	"web/static/js/dashboard.js",
	"web/static/js/day-detail.js",
	"web/static/js/auth.js",
	"web/static/js/banners.js",
	"web/static/js/services.js",
	"web/static/js/matrix.js",
	"web/static/js/blocks.js",
	"web/static/js/app-init.js",
}

// Admin JS bundle — lazy-loaded for authenticated users only
var adminJSFiles = []string{
	"web/static/js/admin-ui.js",
	"web/static/js/service-mgmt.js",
	"web/static/js/settings-tab.js",
	"web/static/js/logs-tab.js",
}

// InitBundles reads and concatenates asset files into in-memory bundles at startup.
// This eliminates CSS @import waterfalls and reduces HTTP requests from 24+ to 3-4.
func InitBundles() {
	Bundles = make(map[string]*Bundle)

	Bundles["/static/css/public-bundle.css"] = buildBundle(publicCSSFiles, "text/css; charset=utf-8")
	Bundles["/static/css/admin-bundle.css"] = buildBundle(adminCSSFiles, "text/css; charset=utf-8")
	Bundles["/static/js/public-bundle.js"] = buildBundle(publicJSFiles, "application/javascript; charset=utf-8")
	Bundles["/static/js/admin-bundle.js"] = buildBundle(adminJSFiles, "application/javascript; charset=utf-8")

	log.Printf("Asset bundles built: %d CSS public (%d bytes), %d CSS admin (%d bytes), %d JS public (%d bytes), %d JS admin (%d bytes)",
		len(publicCSSFiles), len(Bundles["/static/css/public-bundle.css"].Content),
		len(adminCSSFiles), len(Bundles["/static/css/admin-bundle.css"].Content),
		len(publicJSFiles), len(Bundles["/static/js/public-bundle.js"].Content),
		len(adminJSFiles), len(Bundles["/static/js/admin-bundle.js"].Content),
	)
}

// buildBundle concatenates files with minimal cleanup (collapse blank lines).
// Gzip middleware handles the actual compression, so we keep the bundle readable.
func buildBundle(files []string, contentType string) *Bundle {
	var sb strings.Builder

	// UTF-8 BOM (EF BB BF) — must be stripped from each file before concatenation,
	// otherwise BOMs end up mid-stream and break CSS/JS parsing in browsers.
	bom := []byte{0xEF, 0xBB, 0xBF}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("Bundle: failed to read %s: %v", f, err)
		}
		// Strip UTF-8 BOM if present
		if len(data) >= 3 && data[0] == bom[0] && data[1] == bom[1] && data[2] == bom[2] {
			data = data[3:]
		}
		// Section marker for debugging (cheap, compresses to nothing with gzip)
		sb.WriteString("/* === ")
		sb.WriteString(filepath.Base(f))
		sb.WriteString(" === */\n")
		sb.Write(data)
		sb.WriteString("\n")
	}

	content := collapseBlankLines(sb.String())
	raw := []byte(content)
	hash := sha256.Sum256(raw)
	etag := fmt.Sprintf(`"%x"`, hash[:8])

	return &Bundle{
		Content:     raw,
		ContentType: contentType,
		ETag:        etag,
	}
}

// collapseBlankLines reduces consecutive blank lines to a single one,
// shrinking the uncompressed bundle without risky transformations.
func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	result := make([]string, 0, len(lines))
	blankCount := 0

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 1 {
				result = append(result, line)
			}
		} else {
			blankCount = 0
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// HandleBundle serves pre-built asset bundles from memory with ETag caching
func HandleBundle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bundle, ok := Bundles[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("ETag", bundle.ETag)
		w.Header().Set("Cache-Control", "public, no-cache")

		// Support conditional requests
		if match := r.Header.Get("If-None-Match"); match == bundle.ETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", bundle.ContentType)
		w.Write(bundle.Content)
	}
}
