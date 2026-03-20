package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
}

func NewServer(runner Runner, runState RunStateStore, metrics *obs.Metrics, logger *log.Logger) *Server {
	s := &Server{runner: runner, runState: runState, metrics: metrics, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
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

	if err := s.runState.Put(r.Context(), runstate.Run{RunID: runID, Goal: req.Goal, Status: runstate.StatusQueued}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist run")
		return
	}
	if err := s.runState.Put(r.Context(), runstate.Run{RunID: runID, Goal: req.Goal, Status: runstate.StatusRunning}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist run")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	result, err := s.runner.Run(ctx, agent.Input{RunID: runID, Goal: req.Goal, MaxSteps: req.MaxSteps})
	if err != nil {
		s.metrics.Inc("http.run.error")
		msg := "run execution failed"
		if s.logger != nil {
			s.logger.Printf("create run failed run_id=%s err=%v", runID, err)
		}
		_ = s.runState.Put(r.Context(), runstate.Run{RunID: runID, Goal: req.Goal, Status: runstate.StatusFailed, Error: err.Error()})
		writeError(w, http.StatusInternalServerError, msg)
		return
	}

	run := runstate.Run{
		RunID:  runID,
		Goal:   req.Goal,
		Status: runstate.StatusCompleted,
		Final:  result.Final,
		Steps:  result.Steps,
	}
	if result.Status != "completed" {
		run.Status = runstate.StatusFailed
		run.Error = "run terminated without completion"
	}
	if err := s.runState.Put(r.Context(), run); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist run")
		return
	}

	stored, ok, err := s.runState.Get(r.Context(), runID)
	if err != nil || !ok {
		writeError(w, http.StatusInternalServerError, "failed to read persisted run")
		return
	}
	writeJSON(w, http.StatusOK, toRunResponse(stored))
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
