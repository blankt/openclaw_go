package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
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
	return NewServer(rt.Orchestrator, rt.RunState, rt.Metrics, logger)
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
