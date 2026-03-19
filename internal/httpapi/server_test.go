package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"openclaw_go/internal/app"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	rt, err := app.NewRuntime(logger)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return NewServer(rt.Orchestrator, rt.Metrics, logger)
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

func TestCreateRunSuccess(t *testing.T) {
	srv := newTestServer(t)

	body, _ := json.Marshal(createRunRequest{Goal: "hello api", MaxSteps: 4})
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}

	var out createRunResponse
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.RunID == "" {
		t.Fatalf("expected run_id")
	}
	if out.Status == "" {
		t.Fatalf("expected status")
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
