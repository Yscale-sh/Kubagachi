package critterforge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFluxGenerateSpritePostsJSONAndReturnsPNG(t *testing.T) {
	wantPNG := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	ref1 := []byte("first-reference")
	ref2 := []byte("second-reference")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/generate" {
			t.Errorf("path = %s, want /generate", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer flux-secret" {
			t.Errorf("Authorization = %q, want bearer token", auth)
		}

		var body struct {
			Prompt     string   `json:"prompt"`
			References []string `json:"references"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Prompt != "draw a tiny kube critter" {
			t.Errorf("prompt = %q", body.Prompt)
		}
		wantRefs := []string{
			base64.StdEncoding.EncodeToString(ref1),
			base64.StdEncoding.EncodeToString(ref2),
		}
		if len(body.References) != len(wantRefs) {
			t.Fatalf("references = %d, want %d", len(body.References), len(wantRefs))
		}
		for i := range wantRefs {
			if body.References[i] != wantRefs[i] {
				t.Errorf("reference %d = %q, want %q", i, body.References[i], wantRefs[i])
			}
		}

		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(wantPNG); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	t.Setenv("FLUX_ENDPOINT", server.URL)
	t.Setenv("FLUX_API_KEY", "flux-secret")
	t.Setenv("FLUX_TIMEOUT_SECONDS", "1")

	model, err := BuildImageModel("flux", "dev", "", "")
	if err != nil {
		t.Fatalf("BuildImageModel: %v", err)
	}
	if id := model.ID(); id != "flux:dev" {
		t.Fatalf("ID = %q, want flux:dev", id)
	}

	got, err := model.GenerateSprite(context.Background(), "draw a tiny kube critter", ref1, ref2)
	if err != nil {
		t.Fatalf("GenerateSprite: %v", err)
	}
	if !bytes.Equal(got, wantPNG) {
		t.Fatalf("png bytes = %v, want %v", got, wantPNG)
	}
}

func TestFluxGenerateSpriteNon2xxIncludesStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusBadGateway)
	}))
	defer server.Close()

	model, err := NewFluxModel(FluxOptions{Endpoint: server.URL, TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("NewFluxModel: %v", err)
	}
	_, err = model.GenerateSprite(context.Background(), "draw")
	if err == nil {
		t.Fatal("GenerateSprite error = nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "502 Bad Gateway") || !strings.Contains(msg, "upstream exploded") {
		t.Fatalf("error = %q, want status and response body", msg)
	}
}

func TestBuildImageModelFluxMalformedEndpoint(t *testing.T) {
	t.Setenv("FLUX_ENDPOINT", "http://[::1")

	_, err := BuildImageModel("flux", "", "", "")
	if err == nil {
		t.Fatal("BuildImageModel error = nil")
	}
	if !strings.Contains(err.Error(), "FLUX_ENDPOINT") {
		t.Fatalf("error = %q, want FLUX_ENDPOINT context", err)
	}
}
