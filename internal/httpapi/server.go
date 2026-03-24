package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"openclaw_go/internal/agent"
	"openclaw_go/internal/obs"
	"openclaw_go/internal/runstate"
)

type Server struct {
	runner   Runner
	runState RunStateStore
	metrics  *obs.Metrics
	logger   *log.Logger
	mux      *http.ServeMux
	jobs     chan queuedJob
	cfg      Config

	closed       atomic.Bool
	jobWG        sync.WaitGroup
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup
}

type queuedJob struct {
	RunID    string
	Goal     string
	MaxSteps int
}

func NewServer(runner Runner, runState RunStateStore, metrics *obs.Metrics, logger *log.Logger) *Server {
	return NewServerWithConfig(runner, runState, metrics, logger, DefaultConfig())
}

func NewServerWithConfig(runner Runner, runState RunStateStore, metrics *obs.Metrics, logger *log.Logger, cfg Config) *Server {
	cfg = cfg.normalize()
	workerCtx, workerCancel := context.WithCancel(context.Background())

	s := &Server{
		runner:       runner,
		runState:     runState,
		metrics:      metrics,
		logger:       logger,
		mux:          http.NewServeMux(),
		jobs:         make(chan queuedJob, cfg.QueueDepth),
		cfg:          cfg,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}
	s.routes()
	for i := 0; i < cfg.WorkerCount; i++ {
		s.workerWG.Add(1)
		go s.runWorker()
	}
	return s
}

// Close drains accepted jobs, then cancels the worker loop.
func (s *Server) Close(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		s.jobWG.Wait()
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.workerCancel()
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		s.workerWG.Wait()
	}()
	select {
	case <-workersDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /v1/metrics", s.handleGetMetrics)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.metrics.Inc("http.healthz")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	s.metrics.Inc("http.run.get")
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}

	run, ok, err := s.runState.Get(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read run")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, toRunResponse(run))
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	s.metrics.Inc("http.run.create")

	if s.closed.Load() {
		writeError(w, http.StatusServiceUnavailable, "server is shutting down")
		return
	}

	var req createRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Goal = strings.TrimSpace(req.Goal)
	if req.Goal == "" {
		writeError(w, http.StatusBadRequest, "goal is required")
		return
	}

	runID := req.RunID
	if runID == "" {
		runID = fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}

	queued := runstate.Run{RunID: runID, Goal: req.Goal, Status: runstate.StatusQueued}
	if err := s.runState.Put(r.Context(), queued); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist run")
		return
	}

	stored, ok, err := s.runState.Get(r.Context(), runID)
	if err != nil || !ok {
		writeError(w, http.StatusInternalServerError, "failed to read persisted run")
		return
	}

	// Count accepted work before enqueue to avoid worker Done/Add races.
	s.jobWG.Add(1)
	select {
	case s.jobs <- queuedJob{RunID: runID, Goal: req.Goal, MaxSteps: req.MaxSteps}:
		s.metrics.Inc("http.run.enqueued")
		writeJSON(w, http.StatusAccepted, toRunResponse(stored))
	default:
		s.jobWG.Done()
		s.metrics.Inc("http.run.rejected")
		_ = s.runState.Put(r.Context(), runstate.Run{RunID: runID, Goal: req.Goal, Status: runstate.StatusFailed, Error: "queue is full"})
		writeError(w, http.StatusServiceUnavailable, "run queue is full")
	}
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, _ *http.Request) {
	s.metrics.Inc("http.metrics.get")
	writeJSON(w, http.StatusOK, metricsResponse{
		Counters:      s.metrics.Snapshot(),
		QueueDepth:    len(s.jobs),
		QueueCapacity: cap(s.jobs),
		WorkerCount:   s.cfg.WorkerCount,
	})
}

func (s *Server) runWorker() {
	defer s.workerWG.Done()
	for {
		select {
		case <-s.workerCtx.Done():
			return
		case job := <-s.jobs:
			s.executeJob(job)
			s.jobWG.Done()
		}
	}
}

func (s *Server) executeJob(job queuedJob) {
	ctx := context.Background()
	s.metrics.Inc("run.started")
	s.metrics.Add("run.inflight", 1)
	defer s.metrics.Add("run.inflight", -1)

	_ = s.runState.Put(ctx, runstate.Run{RunID: job.RunID, Goal: job.Goal, Status: runstate.StatusRunning})

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.RunTimeout)
	defer cancel()

	result, err := s.runner.Run(runCtx, agent.Input{RunID: job.RunID, Goal: job.Goal, MaxSteps: job.MaxSteps})
	if err != nil {
		s.metrics.Inc("http.run.error")
		s.metrics.Inc("run.failed")
		if s.logger != nil && !errors.Is(err, context.DeadlineExceeded) {
			s.logger.Printf("run worker failed run_id=%s err=%v", job.RunID, err)
		}
		_ = s.runState.Put(ctx, runstate.Run{RunID: job.RunID, Goal: job.Goal, Status: runstate.StatusFailed, Error: err.Error()})
		return
	}

	run := runstate.Run{RunID: job.RunID, Goal: job.Goal, Status: runstate.StatusCompleted, Final: result.Final, Steps: result.Steps}
	if result.Status != "completed" {
		run.Status = runstate.StatusFailed
		run.Error = "run terminated without completion"
		s.metrics.Inc("run.failed")
	} else {
		s.metrics.Inc("run.completed")
	}
	_ = s.runState.Put(ctx, run)
}

func toRunResponse(run runstate.Run) runResponse {
	return runResponse{
		RunID:     run.RunID,
		Goal:      run.Goal,
		Status:    string(run.Status),
		Final:     run.Final,
		Steps:     run.Steps,
		Error:     run.Error,
		CreatedAt: run.CreatedAt,
		UpdatedAt: run.UpdatedAt,
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
