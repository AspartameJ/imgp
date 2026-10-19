package util

import (
	"testing"
)

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
