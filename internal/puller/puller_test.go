package puller

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	if isRetryable(nil) {
		t.Error("isRetryable(nil) = true, want false")
	}

	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection timed out")}
	if !isRetryable(netErr) {
		t.Error("net.OpError should be retryable")
	}

	if isRetryable(errors.New("unexpected status code 403 Forbidden")) {
		t.Error("403 should not be retryable")
	}

	if !isRetryable(errors.New("unexpected status code 502 Bad Gateway")) {
		t.Error("5xx should be retryable")
	}
	if !isRetryable(errors.New("unexpected EOF")) {
		t.Error("unexpected EOF should be retryable")
	}
	if !isRetryable(errors.New("connection reset by peer")) {
		t.Error("connection reset should be retryable")
	}
	if !isRetryable(errors.New("connection refused")) {
		t.Error("connection refused should be retryable")
	}
	if !isRetryable(errors.New("TLS handshake error")) {
		t.Error("TLS handshake should be retryable")
	}
	if !isRetryable(errors.New("broken pipe")) {
		t.Error("broken pipe should be retryable")
	}
}

func TestSendEvent(t *testing.T) {
	ch := make(chan int, 1)
	ctx := context.Background()

	if !sendEvent(ctx, ch, 42) {
		t.Error("sendEvent returned false, want true")
	}
	if v := <-ch; v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

func TestSendEvent_CancelledCtx(t *testing.T) {
	ch := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if sendEvent(ctx, ch, 1) {
		t.Error("sendEvent on cancelled ctx returned true, want false")
	}
}

func TestNewPuller(t *testing.T) {
	p := NewPuller("/tmp/cache")
	if p == nil {
		t.Fatal("NewPuller returned nil")
	}
	if p.cacheDir != "/tmp/cache" {
		t.Errorf("cacheDir = %q, want /tmp/cache", p.cacheDir)
	}
}

func TestPullerOptions(t *testing.T) {
	p := NewPuller("/tmp/cache").
		WithNoCache(true).
		WithLayerTimeout(5 * time.Minute).
		WithRetry(3)

	if !p.noCache {
		t.Error("noCache should be true")
	}
	if p.layerTimeout != 5*time.Minute {
		t.Errorf("layerTimeout = %v, want 5m", p.layerTimeout)
	}
	if p.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", p.maxRetries)
	}
}

func TestPullerWithRetryNegative(t *testing.T) {
	p := NewPuller("/tmp/cache").WithRetry(-1)
	if p.maxRetries != 2 {
		t.Errorf("maxRetries should stay 2, got %d", p.maxRetries)
	}
}
