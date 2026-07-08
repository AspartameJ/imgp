package util

import (
	"context"
	"testing"
	"time"
)

func TestBackoff_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Backoff(ctx, 1)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBackoff_Success(t *testing.T) {
	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- Backoff(ctx, 1)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("backoff timed out")
	}
}
