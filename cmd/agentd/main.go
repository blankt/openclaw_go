package main

import (
	"log"
	"os"

	"openclaw_go/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "agentd ", log.LstdFlags|log.Lmicroseconds)
	if err := app.Run(logger); err != nil {
		logger.Fatalf("agentd failed: %v", err)
	}
}
