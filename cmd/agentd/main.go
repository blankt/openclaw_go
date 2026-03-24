package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"openclaw_go/internal/app"
	"openclaw_go/internal/httpapi"
)

func main() {
	logger := log.New(os.Stdout, "agentd ", log.LstdFlags|log.Lmicroseconds)

	rt, err := app.NewRuntime(logger)
	if err != nil {
		logger.Fatalf("runtime init failed: %v", err)
	}

	api := httpapi.NewServerWithConfig(rt.Orchestrator, rt.RunState, rt.Metrics, logger, httpapi.Config{
		QueueDepth:    envIntOrDefault("AGENTD_QUEUE_DEPTH", 128),
		RunTimeout:    envDurationOrDefault("AGENTD_RUN_TIMEOUT", 30*time.Second),
		WorkerCount:   envIntOrDefault("AGENTD_WORKER_COUNT", 1),
		IngressAPIKey: envOrDefault("AGENTD_INGRESS_API_KEY", ""),
		CreateRunRPM:  envIntOrDefault("AGENTD_CREATE_RUN_RPM", 60),
	})
	srv := &http.Server{
		Addr:    envOrDefault("AGENTD_ADDR", ":8080"),
		Handler: api.Handler(),
	}

	go func() {
		logger.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("http server failed: %v", err)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("http shutdown error: %v", err)
	}
	if err := api.Close(shutdownCtx); err != nil {
		logger.Printf("worker shutdown error: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func envIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
