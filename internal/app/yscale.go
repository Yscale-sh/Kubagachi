package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// registerYscale wires GET /api/yscale — a read-only proxy to a yscale central
// so the browser cockpit's "yscale" tab can render the tenant's live burst fleet
// + spend. The bearer token stays server-side (the browser never receives it).
//
// Response shape:
//
//	{ "configured": false }                                  // --yscale-url unset
//	{ "configured": true, "url": ..., "error": "..." }       // upstream unreachable
//	{ "configured": true, "url": ..., "spend": {...},
//	  "bursts": [...], "count": N }                           // ok
//
// "configured:false" (not an HTTP error) lets the tab show a friendly
// "not wired up" state instead of a red failure.
func registerYscale(mux *http.ServeMux, cfg Config) {
	base := strings.TrimRight(cfg.YscaleURL, "/")
	client := &http.Client{Timeout: 8 * time.Second}

	mux.HandleFunc("/api/yscale", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		if base == "" {
			writeJSON(w, map[string]any{"configured": false})
			return
		}
		out := map[string]any{"configured": true, "url": base}

		spend, err := yscaleGet(r.Context(), client, base+"/v1/spend", cfg.YscaleToken)
		if err != nil {
			out["error"] = "spend: " + err.Error()
			writeJSON(w, out)
			return
		}
		out["spend"] = spend

		raw, err := yscaleGet(r.Context(), client, base+"/v1/bursts", cfg.YscaleToken)
		if err != nil {
			out["error"] = "bursts: " + err.Error()
			writeJSON(w, out)
			return
		}
		var bursts struct {
			Bursts json.RawMessage `json:"bursts"`
			Count  int             `json:"count"`
		}
		_ = json.Unmarshal(raw, &bursts)
		if len(bursts.Bursts) == 0 {
			bursts.Bursts = json.RawMessage("[]")
		}
		out["bursts"] = bursts.Bursts
		out["count"] = bursts.Count
		writeJSON(w, out)
	})
}

// yscaleGet does an authenticated GET against a yscale central endpoint and
// returns the raw JSON body (kept opaque so this proxy doesn't couple to
// central's schema — new fields flow through untouched).
func yscaleGet(ctx context.Context, client *http.Client, url, token string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.RawMessage(body), nil
}
