package cmd

import (
	"math"
	"strings"
	"testing"
)

func TestShorten(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 3, "hel"},
		{"hi", 5, "hi"},
		{"", 3, ""},
		{"abc", 0, ""},
		{"abc", -1, ""},
	}
	for _, tt := range tests {
		got := shorten(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("shorten(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.b)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestRenderBar(t *testing.T) {
	bar := renderBar(50, 100, 10)
	if !strings.Contains(bar, "50%") {
		t.Errorf("renderBar(50, 100, 10) = %q, want 50%%", bar)
	}
	if len(bar) < 5 {
		t.Errorf("renderBar returned too short: %q", bar)
	}

	zero := renderBar(0, 0, 10)
	if zero != "░░░░░░░░░░" {
		t.Errorf("renderBar(0, 0, 10) = %q, want empty bar", zero)
	}

	full := renderBar(100, 100, 10)
	if !strings.Contains(full, "100%") {
		t.Errorf("renderBar(100, 100, 10) = %q, want 100%%", full)
	}

	clamped := renderBar(math.MaxInt64, 100, 10)
	if !strings.Contains(clamped, "100%") {
		t.Errorf("renderBar overflow = %q, want clamped 100%%", clamped)
	}
}

func TestIsValidArch(t *testing.T) {
	valid := []string{"amd64", "arm64", "386", "arm", "s390x", "wasm"}
	for _, a := range valid {
		if !isValidArch(a) {
			t.Errorf("isValidArch(%q) = false, want true", a)
		}
	}
	invalid := []string{"x86_64", "aarch64", "", "sparc"}
	for _, a := range invalid {
		if isValidArch(a) {
			t.Errorf("isValidArch(%q) = true, want false", a)
		}
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
		got := archSuggestion(tt.arch)
		if got != tt.want {
			t.Errorf("archSuggestion(%q) = %q, want %q", tt.arch, got, tt.want)
		}
	}
}
