// Command critterview serves the static sprite gallery at viewer/index.html
// along with the generated critters/ directory, so a browser can fetch the
// manifest and PNGs without file:// security restrictions. It is a
// development tool for critterforge output — the product UI lives in
// `kubagachi --web`.
//
// Usage:
//
//	go run ./cmd/critterview
//	go run ./cmd/critterview --addr :9000 --root .
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jakenesler/kubagachi/internal/sprites"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	root := flag.String("root", ".", "directory to serve (must contain viewer/ and critters/)")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "viewer", "index.html")); err != nil {
		log.Fatalf("viewer/index.html not found under %s: %v", abs, err)
	}
	crittersDir := filepath.Join(abs, "critters")
	if _, err := os.Stat(crittersDir); err != nil {
		log.Fatalf("critters/ not found under %s", abs)
	}

	mux := http.NewServeMux()
	// Both /critters/ (generated PNGs) and /viewer/ (the gallery itself)
	// change frequently. Disable caching so refreshes always pick up the
	// latest assets.
	noCache := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store, must-revalidate")
			h.ServeHTTP(w, r)
		})
	}
	fs := http.FileServer(http.Dir(abs))
	mux.Handle("/critters/", noCache(fs))
	mux.Handle("/viewer/", noCache(fs))

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/critters", func(w http.ResponseWriter, r *http.Request) {
		list, err := sprites.Scan(crittersDir)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"states": sprites.States, "critters": list})
	})
	mux.HandleFunc("/api/latest", func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if v := r.URL.Query().Get("limit"); v != "" {
			fmt.Sscanf(v, "%d", &limit)
			if limit <= 0 || limit > 500 {
				limit = 20
			}
		}
		items, err := sprites.ScanLatest(crittersDir, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"items": items})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/viewer/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	url := fmt.Sprintf("http://localhost%s/", *addr)
	log.Printf("critterview serving %s (root: %s)", url, abs)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
