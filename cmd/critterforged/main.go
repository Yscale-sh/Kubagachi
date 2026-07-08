// Command critterforged exposes the critterforge keyed-sheet generator as an
// HTTP job service for paid marketplace sprite-sheet generation.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/yscale-sh/kubagachi/pkg/critterforge"
)

const (
	defaultPort              = "8080"
	maxReferences            = 3
	maxReferenceBytes        = 10 << 20
	maxEncodedReferenceBytes = (maxReferenceBytes + 2) / 3 * 4
	maxRequestBytes          = maxReferences*maxEncodedReferenceBytes + 1<<20
	sheetProvider            = "gemini"
	sheetQuality             = "medium"
	defaultGenConcurrency    = 4
	defaultJobMaxAttempts    = 5
)

var (
	jobStateTTL           = time.Hour
	requeueBackoffBase    = 250 * time.Millisecond
	requeueBackoffMax     = 5 * time.Second
	redisInFlightTimeout  = 3 * time.Minute
	redisRecoverInterval  = 30 * time.Second
	redisPopTimeout       = 2 * time.Second
	errNoJobs             = errors.New("no jobs available")
	inMemoryQueueCapacity = 1024
	redisQueueKey         = "critterforge:v1:jobs"
	redisProcessingKey    = "critterforge:v1:jobs:processing"
	redisInflightHash     = "critterforge:v1:jobs:inflight"
	redisDequeueFailures  = "critterforge:v1:jobs:dequeue-failures"
	redisJobKeyPrefix     = "critterforge:v1:job:"
)

var safeNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

type jobStatus string

const (
	statusPending jobStatus = "pending"
	statusRunning jobStatus = "running"
	statusDone    jobStatus = "done"
	statusError   jobStatus = "error"
)

// generateRequest is the JSON body for POST /v1/generate. Callers must provide
// name and at least one of description or technology; references are up to
// three base64 PNG/JPEG images, optionally sent as data URLs.
type generateRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Technology   string   `json:"technology,omitempty"`
	References   []string `json:"references,omitempty"`
	Personality  string   `json:"personality,omitempty"`
	Palette      string   `json:"palette,omitempty"`
	Style        string   `json:"style,omitempty"`
	Instructions string   `json:"instructions,omitempty"`

	referencePNGs [][]byte
}

type createJobResponse struct {
	JobID  string    `json:"jobId"`
	Status jobStatus `json:"status"`
}

type jobResponse struct {
	Status jobStatus       `json:"status"`
	Error  string          `json:"error,omitempty"`
	Result *generateResult `json:"result,omitempty"`
}

type generateResult struct {
	SpriteSheetPNGBase64 string         `json:"spriteSheetPngBase64"`
	States               []string       `json:"states"`
	Width                int            `json:"width"`
	Height               int            `json:"height"`
	Meta                 map[string]any `json:"meta,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type persistedRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	Technology    string   `json:"technology,omitempty"`
	References    []string `json:"references,omitempty"`
	Personality   string   `json:"personality,omitempty"`
	Palette       string   `json:"palette,omitempty"`
	Style         string   `json:"style,omitempty"`
	Instructions  string   `json:"instructions,omitempty"`
	ReferencePNGs [][]byte `json:"referencePngs,omitempty"`
}

type jobData struct {
	Status   jobStatus         `json:"status"`
	Error    string            `json:"error"`
	Attempts int               `json:"attempts"`
	Result   *generateResult   `json:"result,omitempty"`
	Request  *persistedRequest `json:"request"`
}

type jobStore interface {
	create(context.Context, string, generateRequest) error
	get(context.Context, string) (jobResponse, bool, error)
	markRunning(context.Context, string) error
	markDone(context.Context, string, *generateResult) error
	markError(context.Context, string, error) error
	enqueue(context.Context, string) error
	requeue(context.Context, string) error
	incrementAttempts(context.Context, string) (int, error)
	release(context.Context, string)
	pop(context.Context) (queuedJob, error)
	stop()
}

type queuedJob struct {
	jobID    string
	request  generateRequest
	attempts int
}

type inMemoryJobStore struct {
	mu    sync.RWMutex
	jobs  map[string]*jobData
	queue chan string
}

type redisJobStore struct {
	client        *redis.Client
	logger        *log.Logger
	recoverCtx    context.Context
	recoverCancel context.CancelFunc
	stopCh        chan struct{}
	stopOnce      sync.Once
}

func newInMemoryJobStore() *inMemoryJobStore {
	return &inMemoryJobStore{
		jobs:  map[string]*jobData{},
		queue: make(chan string, inMemoryQueueCapacity),
	}
}

func (r persistedRequest) toGenerateRequest() generateRequest {
	return generateRequest{
		Name:          r.Name,
		Description:   r.Description,
		Technology:    r.Technology,
		References:    r.References,
		Personality:   r.Personality,
		Palette:       r.Palette,
		Style:         r.Style,
		Instructions:  r.Instructions,
		referencePNGs: append([][]byte(nil), r.ReferencePNGs...),
	}
}

func toPersistedRequest(req generateRequest) persistedRequest {
	return persistedRequest{
		Name:          req.Name,
		Description:   req.Description,
		Technology:    req.Technology,
		References:    req.References,
		Personality:   req.Personality,
		Palette:       req.Palette,
		Style:         req.Style,
		Instructions:  req.Instructions,
		ReferencePNGs: append([][]byte(nil), req.referencePNGs...),
	}
}

func (j *jobData) response() jobResponse {
	return jobResponse{
		Status: j.Status,
		Error:  j.Error,
		Result: j.Result,
	}
}

func (s *inMemoryJobStore) create(ctx context.Context, id string, req generateRequest) error {
	persisted := toPersistedRequest(req)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id] = &jobData{
		Status:  statusPending,
		Request: &persisted,
	}
	return nil
}

func (s *inMemoryJobStore) markRunning(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.Status = statusRunning
		return nil
	}
	return fmt.Errorf("job %s not found", id)
}

func (s *inMemoryJobStore) markDone(_ context.Context, id string, result *generateResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.Status = statusDone
		j.Error = ""
		j.Result = result
		return nil
	}
	return fmt.Errorf("job %s not found", id)
}

func (s *inMemoryJobStore) markError(_ context.Context, id string, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.Status = statusError
		j.Error = err.Error()
		j.Result = nil
		return nil
	}
	return fmt.Errorf("job %s not found", id)
}

func (s *inMemoryJobStore) incrementAttempts(_ context.Context, id string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[id]
	if j == nil {
		return 0, fmt.Errorf("job %s not found", id)
	}
	j.Attempts++
	return j.Attempts, nil
}

func (s *inMemoryJobStore) get(_ context.Context, id string) (jobResponse, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return jobResponse{}, false, nil
	}
	return j.response(), true, nil
}

func (s *inMemoryJobStore) enqueue(_ context.Context, id string) error {
	select {
	case s.queue <- id:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("job queue is full")
	}
}

func (s *inMemoryJobStore) requeue(ctx context.Context, id string) error {
	return s.enqueue(ctx, id)
}

func (s *inMemoryJobStore) pop(ctx context.Context) (queuedJob, error) {
	select {
	case <-ctx.Done():
		return queuedJob{}, ctx.Err()
	case id := <-s.queue:
		s.mu.RLock()
		j, ok := s.jobs[id]
		s.mu.RUnlock()
		if !ok || j.Request == nil {
			return queuedJob{}, fmt.Errorf("job %s payload missing", id)
		}
		return queuedJob{
			jobID:    id,
			request:  j.Request.toGenerateRequest(),
			attempts: j.Attempts,
		}, nil
	}
}

func (s *inMemoryJobStore) stop() {}

func (s *inMemoryJobStore) release(_ context.Context, id string) {}

func newRedisJobStore(url string, logger *log.Logger) (*redisJobStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithCancel(context.Background())
	store := &redisJobStore{
		client:        client,
		logger:        logger,
		recoverCtx:    ctx,
		recoverCancel: cancel,
		stopCh:        make(chan struct{}),
	}
	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	store.recoverInFlightJobs(ctx, true)
	go store.recoverLoop()
	return store, nil
}

func (s *redisJobStore) stop() {
	s.recoverCancel()
	s.stopOnce.Do(func() {
		close(s.stopCh)
		_ = s.client.Close()
	})
}

func (s *redisJobStore) create(ctx context.Context, id string, req generateRequest) error {
	data := toPersistedRequest(req)
	record := jobData{
		Status:  statusPending,
		Request: &data,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, redisJobKey(id), payload, jobStateTTL)
	pipe.RPush(ctx, redisQueueKey, id)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisJobStore) markRunning(ctx context.Context, id string) error {
	return s.withJobLock(ctx, id, func(job *jobData) {
		job.Status = statusRunning
		job.Error = ""
	})
}

func (s *redisJobStore) markDone(ctx context.Context, id string, result *generateResult) error {
	return s.withJobLock(ctx, id, func(job *jobData) {
		job.Status = statusDone
		job.Error = ""
		job.Result = result
	})
}

func (s *redisJobStore) markError(ctx context.Context, id string, err error) error {
	return s.withJobLock(ctx, id, func(job *jobData) {
		job.Status = statusError
		job.Error = err.Error()
		job.Result = nil
	})
}

func (s *redisJobStore) incrementAttempts(ctx context.Context, id string) (int, error) {
	var attempts int
	err := s.withJobLock(ctx, id, func(job *jobData) {
		job.Attempts++
		attempts = job.Attempts
	})
	return attempts, err
}

func (s *redisJobStore) get(ctx context.Context, id string) (jobResponse, bool, error) {
	record, ok, err := s.readJob(ctx, id)
	if err != nil {
		return jobResponse{}, false, err
	}
	if !ok {
		return jobResponse{}, false, nil
	}
	return record.response(), true, nil
}

func (s *redisJobStore) enqueue(ctx context.Context, id string) error {
	// Redis create persists and queues in one transaction; keep this for jobStore compatibility.
	return nil
}

func (s *redisJobStore) requeue(ctx context.Context, id string) error {
	if err := s.markRunning(ctx, id); err != nil {
		return err
	}
	if err := s.releaseProcessing(ctx, id); err != nil {
		return err
	}
	return s.client.RPush(ctx, redisQueueKey, id).Err()
}

func (s *redisJobStore) release(ctx context.Context, id string) {
	_ = s.releaseProcessing(ctx, id)
}

func (s *redisJobStore) pop(ctx context.Context) (queuedJob, error) {
	for {
		id, err := s.client.BRPopLPush(ctx, redisQueueKey, redisProcessingKey, redisPopTimeout).Result()
		if errors.Is(err, redis.Nil) {
			return queuedJob{}, errNoJobs
		}
		if err != nil {
			return queuedJob{}, err
		}
		record, ok, err := s.readJob(ctx, id)
		if err != nil {
			if recoverErr := s.handleDequeueFailure(ctx, id, err); recoverErr != nil {
				return queuedJob{}, fmt.Errorf("job %s dequeue recovery failed: %w", id, recoverErr)
			}
			return queuedJob{}, err
		}
		if !ok {
			_ = s.releaseProcessing(ctx, id)
			continue
		}
		if record.Status == statusDone || record.Status == statusError {
			_ = s.releaseProcessing(ctx, id)
			continue
		}
		if record.Request == nil {
			err := errors.New("job payload missing")
			if recoverErr := s.handleDequeueFailure(ctx, id, err); recoverErr != nil {
				return queuedJob{}, fmt.Errorf("job %s dequeue recovery failed: %w", id, recoverErr)
			}
			return queuedJob{}, err
		}
		if err := s.touchInFlight(ctx, id); err != nil {
			if recoverErr := s.handleDequeueFailure(ctx, id, err); recoverErr != nil {
				return queuedJob{}, fmt.Errorf("job %s dequeue recovery failed: %w", id, recoverErr)
			}
			return queuedJob{}, err
		}
		return queuedJob{
			jobID:    id,
			request:  record.Request.toGenerateRequest(),
			attempts: record.Attempts,
		}, nil
	}
}

func (s *redisJobStore) recoverLoop() {
	ticker := time.NewTicker(redisRecoverInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.recoverInFlightJobs(s.recoverCtx, false)
		case <-s.stopCh:
			return
		}
	}
}

func (s *redisJobStore) recoverInFlightJobs(ctx context.Context, force bool) {
	ids, err := s.client.LRange(ctx, redisProcessingKey, 0, -1).Result()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.logger.Printf("recover jobs: list processing: %v", err)
		}
		return
	}
	staleThreshold := time.Now().Add(-redisInFlightTimeout).Unix()
	for _, id := range ids {
		lastClaim, err := s.client.HGet(ctx, redisInflightHash, id).Int64()
		if !force && err != nil && !errors.Is(err, redis.Nil) {
			s.logger.Printf("recover job %s: inflight lookup: %v", id, err)
			continue
		}
		if !force && err == nil && lastClaim >= staleThreshold {
			continue
		}
		record, ok, err := s.readJob(ctx, id)
		if err != nil {
			s.logger.Printf("recover job %s: read job: %v", id, err)
			if recoverErr := s.handleDequeueFailure(ctx, id, err); recoverErr != nil {
				s.logger.Printf("recover job %s: dequeue recovery failed: %v", id, recoverErr)
			}
			continue
		}
		if !ok || record.Status == statusDone || record.Status == statusError {
			_ = s.releaseProcessing(ctx, id)
			continue
		}
		if record.Request == nil {
			if recoverErr := s.handleDequeueFailure(ctx, id, errors.New("job payload missing")); recoverErr != nil {
				s.logger.Printf("recover job %s: dequeue recovery failed: %v", id, recoverErr)
			}
			continue
		}
		if err := s.requeue(ctx, id); err != nil {
			s.logger.Printf("recover job %s: requeue failed: %v", id, err)
		}
	}
}

func (s *redisJobStore) readJob(ctx context.Context, id string) (jobData, bool, error) {
	key := redisJobKey(id)
	payload, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return jobData{}, false, nil
	}
	if err != nil {
		return jobData{}, false, err
	}
	var record jobData
	if err := json.Unmarshal(payload, &record); err != nil {
		return jobData{}, false, err
	}
	return record, true, nil
}

func (s *redisJobStore) setJob(ctx context.Context, id string, record jobData) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, redisJobKey(id), payload, jobStateTTL).Err()
}

func (s *redisJobStore) withJobLock(ctx context.Context, id string, mutate func(*jobData)) error {
	record, ok, err := s.readJob(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	mutate(&record)
	return s.setJob(ctx, id, record)
}

func (s *redisJobStore) releaseProcessing(ctx context.Context, id string) error {
	pipe := s.client.TxPipeline()
	_ = pipe.LRem(ctx, redisProcessingKey, 1, id)
	pipe.HDel(ctx, redisInflightHash, id)
	pipe.HDel(ctx, redisDequeueFailures, id)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisJobStore) touchInFlight(ctx context.Context, id string) error {
	return s.client.HSet(ctx, redisInflightHash, id, time.Now().Unix()).Err()
}

func (s *redisJobStore) handleDequeueFailure(ctx context.Context, id string, cause error) error {
	attempts, err := s.recordDequeueFailure(ctx, id)
	if err != nil {
		if requeueErr := s.requeueProcessing(ctx, id); requeueErr != nil {
			return fmt.Errorf("%v; requeue processing: %w", err, requeueErr)
		}
		return err
	}
	if attempts >= defaultJobMaxAttempts {
		if err := s.markDequeueError(ctx, id, attempts, cause); err != nil {
			if requeueErr := s.requeueProcessing(ctx, id); requeueErr != nil {
				return fmt.Errorf("%v; requeue processing: %w", err, requeueErr)
			}
			return err
		}
		return s.releaseProcessing(ctx, id)
	}
	return s.requeueProcessing(ctx, id)
}

func (s *redisJobStore) recordDequeueFailure(ctx context.Context, id string) (int, error) {
	attempts, err := s.incrementAttempts(ctx, id)
	if err == nil {
		return attempts, nil
	}
	fallbackAttempts, fallbackErr := s.client.HIncrBy(ctx, redisDequeueFailures, id, 1).Result()
	if fallbackErr != nil {
		return 0, fmt.Errorf("increment dequeue attempts: %v; increment fallback attempts: %w", err, fallbackErr)
	}
	return int(fallbackAttempts), nil
}

func (s *redisJobStore) markDequeueError(ctx context.Context, id string, attempts int, cause error) error {
	err := fmt.Errorf("dequeue failed after %d attempts: %w", attempts, cause)
	if markErr := s.markError(ctx, id, err); markErr == nil {
		return nil
	}
	record := jobData{
		Status:   statusError,
		Error:    err.Error(),
		Attempts: attempts,
	}
	return s.setJob(ctx, id, record)
}

func (s *redisJobStore) requeueProcessing(ctx context.Context, id string) error {
	pipe := s.client.TxPipeline()
	_ = pipe.LRem(ctx, redisProcessingKey, 1, id)
	pipe.HDel(ctx, redisInflightHash, id)
	pipe.RPush(ctx, redisQueueKey, id)
	_, err := pipe.Exec(ctx)
	return err
}

func redisJobKey(id string) string {
	return redisJobKeyPrefix + id
}

type server struct {
	token    string
	model    critterforge.ImageModel
	jobs     jobStore
	ctx      context.Context
	workers  int
	maxRetry int
	logger   *log.Logger
	forgeLog logfLogger
}

type logfLogger struct {
	logger *log.Logger
}

func (l logfLogger) Logf(format string, args ...any) {
	l.logger.Printf(format, args...)
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("critterforged: %v", err)
	}
}

func run() error {
	_ = godotenv.Load()

	token := strings.TrimSpace(os.Getenv("FORGE_TOKEN"))
	if token == "" {
		return errors.New("FORGE_TOKEN is required")
	}
	if strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) == "" {
		return errors.New("GEMINI_API_KEY is required")
	}

	model, err := critterforge.BuildImageModel(sheetProvider, "", critterforge.SheetSize, sheetQuality)
	if err != nil {
		return err
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	workers := readIntEnv("GEN_CONCURRENCY", defaultGenConcurrency)
	maxRetry := defaultJobMaxAttempts

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := log.New(os.Stdout, "critterforged: ", log.LstdFlags)
	var jobs jobStore
	if redisURL == "" {
		logger.Printf("WARNING: REDIS_URL is unset; using single-process in-memory jobs. Horizontal scaling requires shared Redis; run one replica or configure cache: redis.")
		jobs = newInMemoryJobStore()
	} else {
		j, err := newRedisJobStore(redisURL, logger)
		if err != nil {
			return err
		}
		jobs = j
	}
	app := &server{
		token:    token,
		model:    model,
		jobs:     jobs,
		ctx:      ctx,
		workers:  workers,
		maxRetry: maxRetry,
		logger:   logger,
		forgeLog: logfLogger{logger: logger},
	}
	defer app.jobs.stop()
	app.startWorkers()

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Printf("shutdown: %v", err)
		}
	}()

	logger.Printf("listening on :%s", port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/generate", s.handleGenerate)
	mux.HandleFunc("/v1/jobs/", s.handleJob)
	return mux
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	var req generateRequest
	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.normalize()
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	references, err := decodeReferences(req.References)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.referencePNGs = references

	jobID, err := newJobID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create job")
		return
	}

	if err := s.jobs.create(s.ctx, jobID, req); err != nil {
		s.logger.Printf("job %s create failed: %v", jobID, err)
		writeError(w, http.StatusInternalServerError, "could not create job")
		return
	}
	if err := s.jobs.enqueue(s.ctx, jobID); err != nil {
		s.logger.Printf("job %s enqueue failed: %v", jobID, err)
		writeError(w, http.StatusInternalServerError, "could not queue job")
		return
	}

	writeJSON(w, http.StatusAccepted, createJobResponse{
		JobID:  jobID,
		Status: statusPending,
	})
}

func (s *server) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	if jobID == "" || strings.Contains(jobID, "/") {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	resp, ok, err := s.jobs.get(s.ctx, jobID)
	if err != nil {
		s.logger.Printf("job %s lookup failed: %v", jobID, err)
		writeError(w, http.StatusInternalServerError, "could not load job")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) authorize(w http.ResponseWriter, r *http.Request) bool {
	header := r.Header.Get("Authorization")
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="critterforge"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (s *server) runJob(task queuedJob) {
	if err := s.jobs.markRunning(s.ctx, task.jobID); err != nil {
		s.logger.Printf("job %s cannot mark running: %v", task.jobID, err)
		return
	}
	defer s.jobs.release(s.ctx, task.jobID)

	for {
		attempt, err := s.jobs.incrementAttempts(s.ctx, task.jobID)
		if err != nil {
			s.logger.Printf("job %s cannot increment attempt: %v", task.jobID, err)
			return
		}

		result, err := s.generate(s.ctx, task.jobID, task.request)
		if err == nil {
			if e := s.jobs.markDone(s.ctx, task.jobID, result); e != nil {
				s.logger.Printf("job %s done persisted failed: %v", task.jobID, e)
			}
			return
		}
		retriable := critterforge.IsRetryableError(err)
		s.logger.Printf("job %s attempt %d failed: %v", task.jobID, attempt, err)
		if !retriable || attempt >= s.maxRetry {
			if e := s.jobs.markError(s.ctx, task.jobID, err); e != nil {
				s.logger.Printf("job %s error persisted failed: %v", task.jobID, e)
			}
			return
		}
		if err := sleepWithContext(s.ctx, retryBackoff(attempt)); err != nil {
			if e := s.jobs.markError(s.ctx, task.jobID, err); e != nil {
				s.logger.Printf("job %s canceled before requeue: %v", task.jobID, e)
			}
			return
		}
		if err := s.jobs.requeue(s.ctx, task.jobID); err != nil {
			s.logger.Printf("job %s requeue failed: %v", task.jobID, err)
			if e := s.jobs.markError(s.ctx, task.jobID, err); e != nil {
				s.logger.Printf("job %s error persisted failed: %v", task.jobID, e)
			}
			return
		}
		return
	}
}

func (s *server) startWorkers() {
	if s.workers < 1 {
		s.workers = 1
	}
	for i := 0; i < s.workers; i++ {
		go s.workerLoop(i)
	}
}

func (s *server) workerLoop(_ int) {
	for {
		task, err := s.jobs.pop(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			if errors.Is(err, errNoJobs) {
				continue
			}
			s.logger.Printf("worker loop dequeue error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		s.runJob(task)
	}
}

func retryBackoff(attempt int) time.Duration {
	delay := requeueBackoffBase << (attempt - 1)
	if delay > requeueBackoffMax {
		return requeueBackoffMax
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readIntEnv(name string, def int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return def
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	if parsed < 1 {
		return def
	}
	return parsed
}

func (s *server) generate(ctx context.Context, jobID string, req generateRequest) (*generateResult, error) {
	outputDir, err := os.MkdirTemp("", "critterforged-"+jobID+"-")
	if err != nil {
		return nil, fmt.Errorf("create temp output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)

	manifest := manifestFromRequest(req)
	specs := critterforge.SheetSpecsFromInput(&manifest)
	if len(specs) != 1 {
		return nil, errors.New("request did not produce one critter sheet spec")
	}

	if err := critterforge.GenerateSheets(ctx, critterforge.SheetOptions{
		Model:     s.model,
		OutputDir: outputDir,
		Force:     true,
		Logger:    s.forgeLog,
	}, specs); err != nil {
		return nil, err
	}

	sheetPath := filepath.Join(outputDir, req.Name, critterforge.SheetFilename)
	pngBytes, err := os.ReadFile(sheetPath)
	if err != nil {
		return nil, fmt.Errorf("read generated sheet: %w", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode generated sheet: %w", err)
	}

	return &generateResult{
		SpriteSheetPNGBase64: base64.StdEncoding.EncodeToString(pngBytes),
		States:               append([]string(nil), critterforge.StatusOrder...),
		Width:                cfg.Width,
		Height:               cfg.Height,
		Meta: map[string]any{
			"name":          req.Name,
			"technology":    req.Technology,
			"provider":      sheetProvider,
			"model":         s.model.ID(),
			"quality":       sheetQuality,
			"sheetFilename": critterforge.SheetFilename,
		},
	}, nil
}

func manifestFromRequest(req generateRequest) critterforge.InputManifest {
	subject := req.Technology
	if subject == "" {
		subject = req.Description
	}

	mascot := subject
	visualRole := fmt.Sprintf("%s Kubernetes service/pod mascot", subject)
	visualDesign := []string{fmt.Sprintf("visual identity inspired by %s", subject)}
	if req.Description != "" {
		mascot = req.Description
		visualRole = fmt.Sprintf("Kubernetes service/pod mascot for %s. Project brief: %s", req.Name, req.Description)
		visualDesign = []string{"project brief: " + req.Description}
		if req.Technology != "" {
			visualDesign = append(visualDesign, "technology hint: "+req.Technology)
		}
	}
	if req.Palette != "" {
		visualDesign = append(visualDesign, "palette: "+req.Palette)
	}
	if req.Style != "" {
		visualDesign = append(visualDesign, "style: "+req.Style)
	}

	return critterforge.InputManifest{
		Critters: []critterforge.InputCritter{{
			Name:          req.Name,
			Description:   req.Description,
			Mascot:        mascot,
			Personality:   req.Personality,
			VisualRole:    visualRole,
			VisualDesign:  visualDesign,
			Instructions:  req.Instructions,
			ReferencePNGs: req.referencePNGs,
		}},
	}
}

func (r *generateRequest) normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Technology = strings.TrimSpace(r.Technology)
	r.Personality = strings.TrimSpace(r.Personality)
	r.Palette = strings.TrimSpace(r.Palette)
	r.Style = strings.TrimSpace(r.Style)
	r.Instructions = strings.TrimSpace(r.Instructions)
	for i := range r.References {
		r.References[i] = strings.TrimSpace(r.References[i])
	}
}

func (r generateRequest) validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	if !safeNameRE.MatchString(r.Name) {
		return errors.New("name must start with a letter or digit and contain only letters, digits, dots, underscores, or dashes")
	}
	if r.Description == "" && r.Technology == "" {
		return errors.New("description or technology is required")
	}
	return nil
}

var errReferenceTooLarge = errors.New("reference image is too large")

func decodeReferences(references []string) ([][]byte, error) {
	if len(references) > maxReferences {
		return nil, fmt.Errorf("references must contain at most %d images", maxReferences)
	}
	decoded := make([][]byte, 0, len(references))
	for _, ref := range references {
		ref = stripDataURLPrefix(ref)
		if ref == "" {
			continue
		}
		data, err := decodeReference(ref)
		if err != nil {
			if errors.Is(err, errReferenceTooLarge) {
				return nil, fmt.Errorf("reference images must be %d bytes or smaller", maxReferenceBytes)
			}
			continue
		}
		if len(data) == 0 {
			continue
		}
		decoded = append(decoded, data)
	}
	return decoded, nil
}

func stripDataURLPrefix(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToLower(ref), "data:") {
		if i := strings.Index(ref, ","); i >= 0 {
			return strings.TrimSpace(ref[i+1:])
		}
		return ""
	}
	return ref
}

func decodeReference(ref string) ([]byte, error) {
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(ref))
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(decoder, maxReferenceBytes+1)); err != nil {
		return nil, err
	}
	if buf.Len() > maxReferenceBytes {
		return nil, errReferenceTooLarge
	}
	return buf.Bytes(), nil
}

func newJobID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
