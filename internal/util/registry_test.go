package util

import "testing"

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
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeRegistry(tt.reg); got != tt.want {
			t.Errorf("NormalizeRegistry(%q) = %q, want %q", tt.reg, got, tt.want)
		}
	}
}
