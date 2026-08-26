package main

import (
	"context"
	"testing"
)

func TestStartStdioServerRunsOnlyStdioRunner(t *testing.T) {
	runs := 0
	err := startStdioServer(context.Background(), func(context.Context) error {
		runs++
		return nil
	})
	if err != nil {
		t.Fatalf("startStdioServer() error = %v", err)
	}
	if runs != 1 {
		t.Fatalf("stdio runner calls = %d, want 1", runs)
	}
}
