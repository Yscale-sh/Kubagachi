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
