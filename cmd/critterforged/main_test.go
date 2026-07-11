package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/yscale-sh/kubagachi/pkg/critterforge"
)

type fakeImageModel struct {
	png        []byte
	mu         sync.Mutex
	prompts    []string
	references [][][]byte
}

func (m *fakeImageModel) GenerateSprite(_ context.Context, prompt string, references ...[]byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	refCopy := make([][]byte, len(references))
	for i, ref := range references {
		refCopy[i] = append([]byte(nil), ref...)
	}
	m.prompts = append(m.prompts, prompt)
	m.references = append(m.references, refCopy)
	return m.png, nil
}

func (m *fakeImageModel) ID() string {
	return "fake:model"
}

func (m *fakeImageModel) lastCall() (string, [][]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.prompts) == 0 {
		return "", nil
	}
	return m.prompts[len(m.prompts)-1], m.references[len(m.references)-1]
}

func TestHealthz(t *testing.T) {
	app := testServer(t)

	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Fatalf("body = %s", got)
	}
}

func TestGenerateRequiresAuthorization(t *testing.T) {
	app := testServer(t)

	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{"name":"redis","technology":"Redis"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGenerateJobDoneResponse(t *testing.T) {
	app := testServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{
		"name":"redis-demo",
		"technology":"Redis",
		"personality":"fast, watchful",
		"palette":"redis red, black, white",
		"style":"chunky 16-bit pixel art",
		"instructions":"make the mascot read as infrastructure tooling"
	}`))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created createJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.JobID == "" {
		t.Fatal("jobId is empty")
	}
	if created.Status != statusPending {
		t.Fatalf("created status = %s, want %s", created.Status, statusPending)
	}

	resp := waitForJob(t, app, created.JobID)
	if resp.Status != statusDone {
		t.Fatalf("job status = %s, want %s; error=%s", resp.Status, statusDone, resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}
	if len(resp.Result.States) != len(critterforge.StatusOrder) {
		t.Fatalf("states len = %d, want %d", len(resp.Result.States), len(critterforge.StatusOrder))
	}
	for i, state := range critterforge.StatusOrder {
		if resp.Result.States[i] != state {
			t.Fatalf("state[%d] = %q, want %q", i, resp.Result.States[i], state)
		}
	}
	if resp.Result.Width <= 0 || resp.Result.Height <= 0 {
		t.Fatalf("invalid dimensions: %dx%d", resp.Result.Width, resp.Result.Height)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Result.SpriteSheetPNGBase64)
	if err != nil {
		t.Fatalf("decode result PNG base64: %v", err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(decoded)); err != nil {
		t.Fatalf("decode result PNG: %v", err)
	}
	if _, ok := resp.Result.Meta["mode"]; ok {
		t.Fatalf("default Kubagachi meta unexpectedly reports a mode: %#v", resp.Result.Meta)
	}
}

func TestGenerateAcceptsDescriptionAndReferences(t *testing.T) {
	model := &fakeImageModel{png: testRawSheetPNG(t)}
	app := testServerWithModel(t, model)

	firstRef := []byte("png reference")
	secondRef := []byte("jpeg reference")
	body, err := json.Marshal(generateRequest{
		Name:        "calendar-bot",
		Description: "A calendar automation app that finds meeting gaps and schedules Kubernetes maintenance windows.",
		References: []string{
			base64.StdEncoding.EncodeToString(firstRef),
			"not base64",
			"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(secondRef),
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created createJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp := waitForJob(t, app, created.JobID)
	if resp.Status != statusDone {
		t.Fatalf("job status = %s, want %s; error=%s", resp.Status, statusDone, resp.Error)
	}

	prompt, refs := model.lastCall()
	if !strings.Contains(prompt, "calendar automation app") {
		t.Fatalf("prompt did not include description: %s", prompt)
	}
	if got, want := len(refs), 2; got != want {
		t.Fatalf("reference count = %d, want %d", got, want)
	}
	if !bytes.Equal(refs[0], firstRef) {
		t.Fatalf("first reference = %q, want %q", refs[0], firstRef)
	}
	if !bytes.Equal(refs[1], secondRef) {
		t.Fatalf("second reference = %q, want %q", refs[1], secondRef)
	}
}

func TestGenerateRejectsTooManyReferences(t *testing.T) {
	app := testServer(t)
	body, err := json.Marshal(generateRequest{
		Name:        "too-many",
		Description: "A project with too many uploaded references.",
		References:  []string{"a", "b", "c", "d"},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestGenerateRequiresDescriptionOrTechnology(t *testing.T) {
	app := testServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{"name":"missing-subject"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestRedisCreateQueuesJobAtomically(t *testing.T) {
	store, cleanup := testRedisJobStore(t)
	defer cleanup()
	ctx := context.Background()
	id := "job-create"

	if err := store.create(ctx, id, generateRequest{Name: "redis-demo", Technology: "Redis"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.enqueue(ctx, id); err != nil {
		t.Fatalf("enqueue compatibility call: %v", err)
	}

	queue := redisList(t, store, redisQueueKey)
	if len(queue) != 1 || queue[0] != id {
		t.Fatalf("queue = %#v, want [%q]", queue, id)
	}
	record, ok, err := store.readJob(ctx, id)
	if err != nil {
		t.Fatalf("read job: %v", err)
	}
	if !ok {
		t.Fatal("job record missing")
	}
	if record.Status != statusPending {
		t.Fatalf("status = %s, want %s", record.Status, statusPending)
	}
	if record.Request == nil || record.Request.Technology != "Redis" {
		t.Fatalf("request = %#v, want Redis technology", record.Request)
	}
}

func TestRedisPopRequeuesUnreadableJobUntilAttemptCap(t *testing.T) {
	store, cleanup := testRedisJobStore(t)
	defer cleanup()
	ctx := context.Background()
	id := "job-bad-json"

	if err := store.client.Set(ctx, redisJobKey(id), []byte("{"), jobStateTTL).Err(); err != nil {
		t.Fatalf("seed corrupt job: %v", err)
	}
	if err := store.client.RPush(ctx, redisQueueKey, id).Err(); err != nil {
		t.Fatalf("seed queue: %v", err)
	}

	for attempt := 1; attempt <= defaultJobMaxAttempts; attempt++ {
		if _, err := store.pop(ctx); err == nil {
			t.Fatalf("pop attempt %d returned nil error", attempt)
		}
		processing := redisList(t, store, redisProcessingKey)
		if len(processing) != 0 {
			t.Fatalf("processing after attempt %d = %#v, want empty", attempt, processing)
		}
		queue := redisList(t, store, redisQueueKey)
		if attempt < defaultJobMaxAttempts {
			if len(queue) != 1 || queue[0] != id {
				t.Fatalf("queue after attempt %d = %#v, want [%q]", attempt, queue, id)
			}
			continue
		}
		if len(queue) != 0 {
			t.Fatalf("queue after terminal attempt = %#v, want empty", queue)
		}
	}

	record, ok, err := store.readJob(ctx, id)
	if err != nil {
		t.Fatalf("read terminal job: %v", err)
	}
	if !ok {
		t.Fatal("terminal job record missing")
	}
	if record.Status != statusError {
		t.Fatalf("status = %s, want %s", record.Status, statusError)
	}
	if record.Attempts != defaultJobMaxAttempts {
		t.Fatalf("attempts = %d, want %d", record.Attempts, defaultJobMaxAttempts)
	}
	if !strings.Contains(record.Error, "dequeue failed after") {
		t.Fatalf("error = %q, want dequeue failure", record.Error)
	}
	failures, err := store.client.HLen(ctx, redisDequeueFailures).Result()
	if err != nil {
		t.Fatalf("count dequeue failures: %v", err)
	}
	if failures != 0 {
		t.Fatalf("dequeue failure entries = %d, want 0", failures)
	}
}

func testServer(t *testing.T) *server {
	t.Helper()
	return testServerWithModel(t, &fakeImageModel{png: testRawSheetPNG(t)})
}

func testServerWithModel(t *testing.T, model *fakeImageModel) *server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	app := &server{
		token:    "test-token",
		model:    model,
		jobs:     newInMemoryJobStore(),
		workers:  4,
		maxRetry: defaultJobMaxAttempts,
		ctx:      context.Background(),
		logger:   logger,
		forgeLog: logfLogger{logger: logger},
	}
	app.startWorkers()
	return app
}

func TestGenerateCharacterModeJob(t *testing.T) {
	model := &fakeImageModel{png: testRawTilesPNG(t, 4, 1)}
	app := testServerWithModel(t, model)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{
		"name":"knight",
		"mode":"character",
		"description":"A tiny armored knight with a blue plume.",
		"action":"walk",
		"emotion":"happy",
		"direction":"left",
		"frameSize":"16x16",
		"style":"chunky 16-bit pixel art"
	}`))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created createJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp := waitForJob(t, app, created.JobID)
	if resp.Status != statusDone {
		t.Fatalf("job status = %s, want %s; error=%s", resp.Status, statusDone, resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}

	wantStates := []string{"frame-1", "frame-2", "frame-3", "frame-4"}
	if len(resp.Result.States) != len(wantStates) {
		t.Fatalf("states = %#v, want %#v", resp.Result.States, wantStates)
	}
	for i, state := range wantStates {
		if resp.Result.States[i] != state {
			t.Fatalf("state[%d] = %q, want %q", i, resp.Result.States[i], state)
		}
	}
	for key, want := range map[string]any{
		"mode":       "character",
		"action":     "walk",
		"emotion":    "happy",
		"direction":  "left",
		"frameSize":  "16x16",
		"frameCount": float64(4),
	} {
		if got := resp.Result.Meta[key]; got != want {
			t.Fatalf("meta[%q] = %#v, want %#v", key, got, want)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Result.SpriteSheetPNGBase64)
	if err != nil {
		t.Fatalf("decode result PNG base64: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("decode result PNG: %v", err)
	}
	if cfg.Width != resp.Result.Width || cfg.Height != resp.Result.Height {
		t.Fatalf("dimensions %dx%d do not match PNG %dx%d", resp.Result.Width, resp.Result.Height, cfg.Width, cfg.Height)
	}
	if cfg.Width != 4*16 || cfg.Height != 16 {
		t.Fatalf("character strip dimensions = %dx%d, want 64x16", cfg.Width, cfg.Height)
	}

	prompt, refs := model.lastCall()
	if len(refs) != 0 {
		t.Fatalf("model received %d references, want 0", len(refs))
	}
	for _, want := range []string{
		"exactly 4 frames total",
		"single horizontal row",
		"walk animation",
		"faces left",
		"happy",
		"16x16 pixel grid",
		"chunky 16-bit pixel art",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestGenerateCharacterModeDefaultsAndReferences(t *testing.T) {
	model := &fakeImageModel{png: testRawTilesPNG(t, 4, 1)}
	app := testServerWithModel(t, model)

	ref := []byte("canonical character png")
	body, err := json.Marshal(generateRequest{
		Name:        "knight",
		Mode:        "character",
		Description: "A tiny armored knight with a blue plume.",
		References:  []string{base64.StdEncoding.EncodeToString(ref)},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created createJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp := waitForJob(t, app, created.JobID)
	if resp.Status != statusDone {
		t.Fatalf("job status = %s, want %s; error=%s", resp.Status, statusDone, resp.Error)
	}
	for key, want := range map[string]any{
		"action":    "idle",
		"emotion":   "neutral",
		"direction": "front",
		"frameSize": "32x32",
	} {
		if got := resp.Result.Meta[key]; got != want {
			t.Fatalf("default meta[%q] = %#v, want %#v", key, got, want)
		}
	}

	prompt, refs := model.lastCall()
	if len(refs) != 1 || !bytes.Equal(refs[0], ref) {
		t.Fatalf("model references = %#v, want the uploaded reference", refs)
	}
	if !strings.Contains(prompt, "CANONICAL CHARACTER") {
		t.Fatalf("prompt is not reference-conditioned:\n%s", prompt)
	}
}

func TestGenerateAssetModeJob(t *testing.T) {
	model := &fakeImageModel{png: testRawTilesPNG(t, 4, 2)}
	app := testServerWithModel(t, model)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(`{
		"name":"dungeon-props",
		"mode":"asset",
		"description":"Dungeon props: torches, barrels, chests, bones.",
		"assetCategory":"items",
		"assetCount":8,
		"assetSize":"16x16",
		"assetStyle":"fantasy"
	}`))
	req.Header.Set("Authorization", "Bearer test-token")
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created createJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp := waitForJob(t, app, created.JobID)
	if resp.Status != statusDone {
		t.Fatalf("job status = %s, want %s; error=%s", resp.Status, statusDone, resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("result is nil")
	}

	if len(resp.Result.States) != 8 {
		t.Fatalf("states len = %d, want 8", len(resp.Result.States))
	}
	for i, state := range resp.Result.States {
		if want := "item-" + strconv.Itoa(i+1); state != want {
			t.Fatalf("state[%d] = %q, want %q", i, state, want)
		}
	}
	for key, want := range map[string]any{
		"mode":          "asset",
		"assetCategory": "items",
		"assetCount":    float64(8),
		"assetSize":     "16x16",
		"assetStyle":    "fantasy",
		"gridColumns":   float64(4),
		"gridRows":      float64(2),
	} {
		if got := resp.Result.Meta[key]; got != want {
			t.Fatalf("meta[%q] = %#v, want %#v", key, got, want)
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Result.SpriteSheetPNGBase64)
	if err != nil {
		t.Fatalf("decode result PNG base64: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("decode result dimensions: %v", err)
	}
	if cfg.Width != 4*16 || cfg.Height != 2*16 {
		t.Fatalf("asset grid dimensions = %dx%d, want 64x32", cfg.Width, cfg.Height)
	}

	prompt, _ := model.lastCall()
	for _, want := range []string{
		"exactly 8 assets total",
		"exactly 4 columns and 2 rows",
		"completely contained inside its own cell",
		"16x16 pixel grid",
		"fantasy",
		"items",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestGenerateModeValidation(t *testing.T) {
	app := testServer(t)

	cases := map[string]string{
		"unknown mode":             `{"name":"x","mode":"banana","description":"d"}`,
		"unknown field":            `{"name":"x","mode":"character","description":"d","frames":9}`,
		"character field no mode":  `{"name":"x","description":"d","action":"walk"}`,
		"asset field no mode":      `{"name":"x","description":"d","assetCount":8}`,
		"character no description": `{"name":"x","mode":"character"}`,
		"character bad action":     `{"name":"x","mode":"character","description":"d","action":"fly"}`,
		"character bad emotion":    `{"name":"x","mode":"character","description":"d","emotion":"smug"}`,
		"character bad direction":  `{"name":"x","mode":"character","description":"d","direction":"up"}`,
		"character bad frame size": `{"name":"x","mode":"character","description":"d","frameSize":"48x48"}`,
		"asset no description":     `{"name":"x","mode":"asset"}`,
		"asset bad category":       `{"name":"x","mode":"asset","description":"d","assetCategory":"weapons"}`,
		"asset bad count":          `{"name":"x","mode":"asset","description":"d","assetCount":5}`,
		"asset bad size":           `{"name":"x","mode":"asset","description":"d","assetSize":"48x48"}`,
		"asset bad style":          `{"name":"x","mode":"asset","description":"d","assetStyle":"vaporwave"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/generate", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			app.routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestCharacterPromptConstruction(t *testing.T) {
	req := generateRequest{
		Name:        "knight",
		Mode:        modeCharacter,
		Description: "A tiny armored knight.",
		Action:      "attack",
		Emotion:     "angry",
		Direction:   "right",
		FrameSize:   "64x64",
	}
	prompt := characterPrompt(req, false)
	for _, want := range []string{
		"exactly 4 frames total",
		"single horizontal row, left to right",
		"evenly sized tile of identical dimensions",
		"completely contained inside its own tile",
		"transparent background with REAL alpha",
		"no checkerboard",
		"nearest-neighbor pixel art",
		"no text, no labels",
		"attack action",
		"faces right",
		"angry",
		"64x64 pixel grid",
		"A tiny armored knight.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "CANONICAL CHARACTER") {
		t.Error("prompt without references claims a canonical reference")
	}
	if !strings.Contains(characterPrompt(req, true), "CANONICAL CHARACTER") {
		t.Error("prompt with references is not reference-conditioned")
	}
}

func TestAssetPromptConstruction(t *testing.T) {
	req := generateRequest{
		Name:          "ui-kit",
		Mode:          modeAsset,
		Description:   "Retro game UI: hearts, coins, buttons.",
		AssetCategory: "ui",
		AssetCount:    16,
		AssetSize:     "32x32",
		AssetStyle:    "sci-fi",
	}
	prompt := assetPrompt(req, 4, 4, false)
	for _, want := range []string{
		"exactly 16 assets total",
		"exactly 4 columns and 4 rows",
		"evenly sized cell of identical dimensions",
		"completely contained inside its own cell",
		"transparent background with REAL alpha",
		"no checkerboard",
		"nearest-neighbor pixel art",
		"no text, no labels",
		"32x32 pixel grid",
		"sci-fi",
		"ui",
		"Retro game UI: hearts, coins, buttons.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "CANONICAL STYLE REFERENCE") {
		t.Error("prompt without references claims a style reference")
	}
	if !strings.Contains(assetPrompt(req, 4, 4, true), "CANONICAL STYLE REFERENCE") {
		t.Error("prompt with references is not reference-conditioned")
	}
}

func TestPersistedRequestRoundTripModes(t *testing.T) {
	req := generateRequest{
		Name:          "knight",
		Description:   "A tiny armored knight.",
		Mode:          modeCharacter,
		Action:        "jump",
		Emotion:       "surprised",
		Direction:     "back",
		FrameSize:     "16x16",
		AssetCategory: "environment",
		AssetCount:    16,
		AssetSize:     "64x64",
		AssetStyle:    "modern",
		referencePNGs: [][]byte{[]byte("ref png")},
	}
	round := toPersistedRequest(req).toGenerateRequest()
	if !reflect.DeepEqual(req, round) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", round, req)
	}

	// The persisted JSON format must also survive marshal/unmarshal (Redis).
	payload, err := json.Marshal(toPersistedRequest(req))
	if err != nil {
		t.Fatalf("marshal persisted request: %v", err)
	}
	var persisted persistedRequest
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("unmarshal persisted request: %v", err)
	}
	if !reflect.DeepEqual(req, persisted.toGenerateRequest()) {
		t.Fatalf("JSON round trip mismatch: %#v", persisted)
	}

	// Legacy records without the new fields decode to the default mode.
	var legacy persistedRequest
	if err := json.Unmarshal([]byte(`{"name":"redis-demo","technology":"Redis"}`), &legacy); err != nil {
		t.Fatalf("unmarshal legacy request: %v", err)
	}
	if got := legacy.toGenerateRequest(); got.Mode != "" || got.AssetCount != 0 {
		t.Fatalf("legacy request gained mode fields: %#v", got)
	}
}

func TestRedisPersistsModeFields(t *testing.T) {
	store, cleanup := testRedisJobStore(t)
	defer cleanup()
	ctx := context.Background()
	id := "job-asset-mode"

	req := generateRequest{
		Name:          "dungeon-props",
		Description:   "Dungeon props.",
		Mode:          modeAsset,
		AssetCategory: "props",
		AssetCount:    4,
		AssetSize:     "32x32",
		AssetStyle:    "retro",
	}
	if err := store.create(ctx, id, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	task, err := store.pop(ctx)
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if task.jobID != id {
		t.Fatalf("popped job = %q, want %q", task.jobID, id)
	}
	got := task.request
	got.referencePNGs = nil
	req.referencePNGs = nil
	if !reflect.DeepEqual(got, req) {
		t.Fatalf("popped request = %#v, want %#v", got, req)
	}
}

func testRedisJobStore(t *testing.T) (*redisJobStore, func()) {
	t.Helper()
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	store := &redisJobStore{
		client: client,
		logger: log.New(io.Discard, "", 0),
	}
	cleanup := func() {
		_ = client.Close()
		redisServer.Close()
	}
	return store, cleanup
}

func redisList(t *testing.T, store *redisJobStore, key string) []string {
	t.Helper()
	values, err := store.client.LRange(context.Background(), key, 0, -1).Result()
	if err != nil {
		t.Fatalf("read redis list %s: %v", key, err)
	}
	return values
}

func waitForJob(t *testing.T, app *server, jobID string) jobResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+jobID, nil)
		req.Header.Set("Authorization", "Bearer test-token")
		app.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("job status code = %d; body=%s", rec.Code, rec.Body.String())
		}
		var resp jobResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode job response: %v", err)
		}
		if resp.Status == statusDone || resp.Status == statusError {
			return resp
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", jobID)
	return jobResponse{}
}

func testRawSheetPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 800, 120))
	for i := 0; i < 8; i++ {
		c := color.NRGBA{R: uint8(30 + i*20), G: 40, B: 90, A: 255}
		for y := 20; y < 100; y++ {
			for x := i*100 + 20; x < i*100+80; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fake sheet: %v", err)
	}
	return buf.Bytes()
}

// testRawTilesPNG fakes a model output with real alpha: cols x rows solid
// blobs, one centered per 100px cell.
func testRawTilesPNG(t *testing.T, cols, rows int) []byte {
	t.Helper()
	const cell = 100
	img := image.NewNRGBA(image.Rect(0, 0, cols*cell, rows*cell))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			blob := color.NRGBA{R: uint8(30 + (r*cols+c)*15), G: 40, B: 90, A: 255}
			for y := r*cell + 20; y < r*cell+80; y++ {
				for x := c*cell + 20; x < c*cell+80; x++ {
					img.SetNRGBA(x, y, blob)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fake tiles: %v", err)
	}
	return buf.Bytes()
}
