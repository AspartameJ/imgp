package util

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("random error"), false},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, true},
		{&net.OpError{Op: "dial", Err: &net.DNSError{IsTimeout: true, Name: "example.com"}}, true},
		{errors.New("unexpected status code 401"), false},
		{errors.New("unexpected status code 403"), false},
		{errors.New("unexpected status code 404"), false},
		{errors.New("unexpected status code 503"), true},
		{errors.New("unexpected EOF"), true},
		{errors.New("connection reset by peer"), true},
		{errors.New("TLS handshake error"), true},
		{errors.New("broken pipe"), true},
	}
	for _, tt := range tests {
		got := IsRetryable(tt.err)
		if got != tt.want {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestIsValidArch(t *testing.T) {
	valid := []string{"amd64", "arm64", "386", "arm", "s390x", "wasm"}
	for _, a := range valid {
		if !IsValidArch(a) {
			t.Errorf("IsValidArch(%q) = false, want true", a)
		}
	}
	invalid := []string{"x86_64", "aarch64", "", "sparc"}
	for _, a := range invalid {
		if IsValidArch(a) {
			t.Errorf("IsValidArch(%q) = true, want false", a)
		}
	}
}

func TestIsConnectivityError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"timeout net.Error", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("timeout")}, true},
		{"dial tcp", fmt.Errorf("dial tcp 1.2.3.4:80: connection refused"), true},
		{"connection reset", fmt.Errorf("read: connection reset by peer"), true},
		{"TLS handshake", fmt.Errorf("TLS handshake error"), true},
		{"broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"i/o timeout", fmt.Errorf("i/o timeout"), true},
		{"HTTP 500", fmt.Errorf("unexpected status code 500"), false},
		{"HTTP 404", fmt.Errorf("unexpected status code 404"), false},
		{"random", fmt.Errorf("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConnectivityError(tt.err); got != tt.want {
				t.Errorf("IsConnectivityError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestArchSuggestion(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"aarch64", "arm64"},
		{"x86_64", "amd64"},
		{"amd", "amd64"},
		{"i386", "386"},
		{"armv7", "arm"},
		{"arm64v8", "arm64"},
		{"riscv", ""},
	}
	for _, tt := range tests {
		got := ArchSuggestion(tt.arch)
		if got != tt.want {
			t.Errorf("ArchSuggestion(%q) = %q, want %q", tt.arch, got, tt.want)
		}
	}
}
