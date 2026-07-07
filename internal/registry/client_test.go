package registry

import (
	"errors"
	"net"
	"testing"
)

func TestNormalizeRegistry(t *testing.T) {
	tests := []struct {
		reg  string
		want string
	}{
		{"docker.io", "docker.io"},
		{"index.docker.io", "docker.io"},
		{"registry-1.docker.io", "docker.io"},
		{"registry-1.docker.io:443", "docker.io"},
		{"k8s.gcr.io", "k8s.gcr.io"},
		{"quay.io:443", "quay.io"},
	}
	for _, tt := range tests {
		got := normalizeRegistry(tt.reg)
		if got != tt.want {
			t.Errorf("normalizeRegistry(%q) = %q, want %q", tt.reg, got, tt.want)
		}
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"linux/amd64", "linux"},
		{"linux/arm64/v8", "linux"},
		{"arm64", "linux"},
		{"", ""},
	}
	for _, tt := range tests {
		p := parsePlatform(tt.input)
		if tt.want == "" {
			if p != nil {
				t.Errorf("parsePlatform(%q) = %+v, want nil", tt.input, p)
			}
			continue
		}
		if p == nil {
			t.Errorf("parsePlatform(%q) = nil", tt.input)
			continue
		}
		if p.OS != tt.want {
			t.Errorf("parsePlatform(%q).OS = %q, want %q", tt.input, p.OS, tt.want)
		}
	}
}

func TestParsePlatform_Invalid(t *testing.T) {
	if p := parsePlatform("/linux"); p != nil {
		t.Errorf("expected nil for /linux, got %+v", p)
	}
	if p := parsePlatform("linux//arm64"); p != nil {
		t.Errorf("expected nil for linux//arm64, got %+v", p)
	}
}

func TestIsRetryableFetch(t *testing.T) {
	if isRetryableFetch(nil) {
		t.Error("isRetryableFetch(nil) = true, want false")
	}

	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	if !isRetryableFetch(netErr) {
		t.Error("net.OpError should be retryable")
	}

	timeoutErr := &net.DNSError{IsTimeout: true, Name: "example.com"}
	wrapped := &net.OpError{Op: "dial", Err: timeoutErr}
	if !isRetryableFetch(wrapped) {
		t.Error("timeout should be retryable")
	}

	if isRetryableFetch(errors.New("unexpected status code 401 Unauthorized")) {
		t.Error("401 should not be retryable")
	}
	if isRetryableFetch(errors.New("unexpected status code 403 Forbidden")) {
		t.Error("403 should not be retryable")
	}
	if isRetryableFetch(errors.New("unexpected status code 404 Not Found")) {
		t.Error("404 should not be retryable")
	}

	if !isRetryableFetch(errors.New("unexpected status code 502 Bad Gateway")) {
		t.Error("502 should be retryable")
	}
	if !isRetryableFetch(errors.New("unexpected EOF")) {
		t.Error("unexpected EOF should be retryable")
	}
	if !isRetryableFetch(errors.New("connection reset by peer")) {
		t.Error("connection reset should be retryable")
	}
	if !isRetryableFetch(errors.New("TLS handshake error")) {
		t.Error("TLS handshake error should be retryable")
	}
	if !isRetryableFetch(errors.New("broken pipe")) {
		t.Error("broken pipe should be retryable")
	}
}
