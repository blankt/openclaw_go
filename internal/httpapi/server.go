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
)

type Server struct {
	runner  Runner
	metrics *obs.Metrics
	logger  *log.Logger
	mux     *http.ServeMux
}

func NewServer(runner Runner, metrics *obs.Metrics, logger *log.Logger) *Server {
	s := &Server{runner: runner, metrics: metrics, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("POST /v1/runs", s.handleCreateRun)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.metrics.Inc("http.healthz")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	result, err := s.runner.Run(ctx, agent.Input{RunID: runID, Goal: req.Goal, MaxSteps: req.MaxSteps})
	if err != nil {
		s.metrics.Inc("http.run.error")
		if s.logger != nil {
			s.logger.Printf("create run failed run_id=%s err=%v", runID, err)
		}
		writeError(w, http.StatusInternalServerError, "run execution failed")
		return
	}

	writeJSON(w, http.StatusOK, createRunResponse{
		RunID:  runID,
		Status: result.Status,
		Final:  result.Final,
		Steps:  result.Steps,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
