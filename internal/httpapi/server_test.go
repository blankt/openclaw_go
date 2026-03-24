package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"openclaw_go/internal/agent"
	"openclaw_go/internal/app"
	"openclaw_go/internal/obs"
	"openclaw_go/internal/runstate"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	rt, err := app.NewRuntime(logger)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	srv := NewServer(rt.Orchestrator, rt.RunState, rt.Metrics, logger)
	t.Cleanup(func() {
		_ = srv.Close(context.Background())
	})
	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
}

func TestCreateRunAccepted(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(createRunRequest{Goal: "hello api", MaxSteps: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", res.Code, res.Body.String())
	}

	var out runResponse
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.RunID == "" {
		t.Fatalf("expected run_id")
	}
	if out.Status != string(runstate.StatusQueued) && out.Status != string(runstate.StatusRunning) {
		t.Fatalf("expected queued/running status on create, got %q", out.Status)
	}
}

func TestCreateRunValidation(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"goal":"   "}`)))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}

func TestGetRunNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/not-exist", nil)
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestCreateRunThenGetCompleted(t *testing.T) {
	srv := newTestServer(t)

	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-get-1","goal":"get after create"}`)))
	resCreate := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resCreate, reqCreate)
	if resCreate.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d", resCreate.Code)
	}

	out := waitForRunStatus(t, srv, "r-get-1", 2*time.Second, string(runstate.StatusCompleted))
	if out.Final == "" {
		t.Fatalf("expected final response content")
	}
}

func TestCreateRunFailureThenGetFailed(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	srv := NewServer(failingRunner{}, store, obs.NewMetrics(), logger)
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-fail-1","goal":"should fail"}`)))
	resCreate := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resCreate, reqCreate)
	if resCreate.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d body=%s", resCreate.Code, resCreate.Body.String())
	}

	out := waitForRunStatus(t, srv, "r-fail-1", 2*time.Second, string(runstate.StatusFailed))
	if out.Error == "" {
		t.Fatalf("expected error message for failed run")
	}
}

type failingRunner struct{}

func (failingRunner) Run(context.Context, agent.Input) (agent.Result, error) {
	return agent.Result{}, errors.New("forced failure")
}

type delayedRunner struct {
	started chan struct{}
	release chan struct{}
}

func (d delayedRunner) Run(ctx context.Context, in agent.Input) (agent.Result, error) {
	select {
	case <-d.started:
		// started already signaled
	default:
		close(d.started)
	}
	select {
	case <-d.release:
	case <-ctx.Done():
		return agent.Result{}, ctx.Err()
	}
	return agent.Result{Status: "completed", Final: in.Goal, Steps: 1}, nil
}

func TestRunTransitionsToRunningBeforeCompletion(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	runner := delayedRunner{started: make(chan struct{}), release: make(chan struct{})}
	srv := NewServer(runner, store, obs.NewMetrics(), logger)
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-running-1","goal":"wait"}`)))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d", res.Code)
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("runner did not start")
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/v1/runs/r-running-1", nil)
	resGet := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resGet, reqGet)
	if resGet.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d", resGet.Code)
	}
	var out runResponse
	if err := json.Unmarshal(resGet.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if out.Status != string(runstate.StatusRunning) {
		t.Fatalf("expected running status, got %q", out.Status)
	}

	close(runner.release)
	waitForRunStatus(t, srv, "r-running-1", 2*time.Second, string(runstate.StatusCompleted))
}

func waitForRunStatus(t *testing.T, srv *Server, runID string, timeout time.Duration, wanted string) runResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil)
		res := httptest.NewRecorder()
		srv.Handler().ServeHTTP(res, req)
		if res.Code == http.StatusOK {
			var out runResponse
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode run response: %v", err)
			}
			if out.Status == wanted {
				return out
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach status %s within %s", runID, wanted, timeout)
	return runResponse{}
}

func TestCreateRejectedAfterClose(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	srv := NewServerWithConfig(failingRunner{}, store, obs.NewMetrics(), logger, Config{QueueDepth: 8, RunTimeout: time.Second})
	if err := srv.Close(context.Background()); err != nil {
		t.Fatalf("close server: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"goal":"after close"}`)))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestCloseWaitsForInFlightJob(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	runner := delayedRunner{started: make(chan struct{}), release: make(chan struct{})}
	srv := NewServerWithConfig(runner, store, obs.NewMetrics(), logger, Config{QueueDepth: 8, RunTimeout: 2 * time.Second})

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-close-1","goal":"wait close"}`)))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d", res.Code)
	}

	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatalf("runner did not start")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- srv.Close(context.Background())
	}()

	select {
	case <-closeDone:
		t.Fatalf("close returned before in-flight job completed")
	case <-time.After(150 * time.Millisecond):
	}

	close(runner.release)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("close did not finish after job release")
	}
}

type timeoutRunner struct{}

func (timeoutRunner) Run(ctx context.Context, _ agent.Input) (agent.Result, error) {
	<-ctx.Done()
	return agent.Result{}, ctx.Err()
}

func TestRunTimeoutFromConfig(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	srv := NewServerWithConfig(timeoutRunner{}, store, obs.NewMetrics(), logger, Config{QueueDepth: 8, RunTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-timeout-1","goal":"timeout"}`)))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d", res.Code)
	}

	out := waitForRunStatus(t, srv, "r-timeout-1", 2*time.Second, string(runstate.StatusFailed))
	if out.Error == "" {
		t.Fatalf("expected timeout error")
	}
}

type parallelRunner struct {
	mu      sync.Mutex
	started int
	gate    chan struct{}
}

func (p *parallelRunner) Run(_ context.Context, in agent.Input) (agent.Result, error) {
	p.mu.Lock()
	p.started++
	p.mu.Unlock()
	<-p.gate
	return agent.Result{Status: "completed", Final: in.Goal, Steps: 1}, nil
}

func (p *parallelRunner) Started() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started
}

func TestMultiWorkerProcessesRunsConcurrently(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	store := runstate.NewInMemoryStore()
	runner := &parallelRunner{gate: make(chan struct{})}
	srv := NewServerWithConfig(runner, store, obs.NewMetrics(), logger, Config{QueueDepth: 8, RunTimeout: 2 * time.Second, WorkerCount: 2})
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(fmt.Sprintf(`{"run_id":"r-par-%d","goal":"parallel"}`, i))))
		res := httptest.NewRecorder()
		srv.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusAccepted {
			t.Fatalf("create expected 202, got %d", res.Code)
		}
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runner.Started() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runner.Started() < 2 {
		t.Fatalf("expected two concurrent worker starts, got %d", runner.Started())
	}

	close(runner.gate)
	waitForRunStatus(t, srv, "r-par-0", 2*time.Second, string(runstate.StatusCompleted))
	waitForRunStatus(t, srv, "r-par-1", 2*time.Second, string(runstate.StatusCompleted))
}

func TestConfigNormalizeDefaultsWorkerCount(t *testing.T) {
	cfg := (Config{QueueDepth: 0, RunTimeout: 0, WorkerCount: 0}).normalize()
	if cfg.QueueDepth != 128 {
		t.Fatalf("unexpected queue depth: %d", cfg.QueueDepth)
	}
	if cfg.RunTimeout != 30*time.Second {
		t.Fatalf("unexpected run timeout: %s", cfg.RunTimeout)
	}
	if cfg.WorkerCount != 1 {
		t.Fatalf("unexpected worker count: %d", cfg.WorkerCount)
	}
}

func TestGetMetricsIncludesSchedulerMetadata(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var out metricsResponse
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if out.QueueCapacity <= 0 {
		t.Fatalf("expected positive queue capacity, got %d", out.QueueCapacity)
	}
	if out.WorkerCount <= 0 {
		t.Fatalf("expected positive worker count, got %d", out.WorkerCount)
	}
	if _, ok := out.Counters["http.metrics.get"]; !ok {
		t.Fatalf("expected http.metrics.get counter in payload")
	}
}

func TestMetricsCountersAfterCompletedRun(t *testing.T) {
	srv := newTestServer(t)

	reqCreate := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"run_id":"r-metrics-1","goal":"metrics"}`)))
	resCreate := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resCreate, reqCreate)
	if resCreate.Code != http.StatusAccepted {
		t.Fatalf("create expected 202, got %d", resCreate.Code)
	}
	waitForRunStatus(t, srv, "r-metrics-1", 2*time.Second, string(runstate.StatusCompleted))

	reqMetrics := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	resMetrics := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resMetrics, reqMetrics)
	if resMetrics.Code != http.StatusOK {
		t.Fatalf("metrics expected 200, got %d", resMetrics.Code)
	}

	var out metricsResponse
	if err := json.Unmarshal(resMetrics.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if out.Counters["run.started"] < 1 {
		t.Fatalf("expected run.started >= 1, got %d", out.Counters["run.started"])
	}
	if out.Counters["run.completed"] < 1 {
		t.Fatalf("expected run.completed >= 1, got %d", out.Counters["run.completed"])
	}
	if out.Counters["run.inflight"] != 0 {
		t.Fatalf("expected run.inflight == 0, got %d", out.Counters["run.inflight"])
	}
}
