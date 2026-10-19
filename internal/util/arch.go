package util

import (
	"strings"
)

// Architecture names reused by the validation list and the suggestion table.
const (
	arch386   = "386"
	archAMD64 = "amd64"
	archARM   = "arm"
	archARM64 = "arm64"
)

// Archs returns all valid CPU architectures in a stable order. It is the
// single source of truth for architecture validation and shell completion.
func Archs() []string {
	return []string{
		arch386, archAMD64, archARM, archARM64, "loong64",
		"mips", "mips64", "mips64le", "mipsle", "ppc64",
		"ppc64le", "riscv64", "s390x", "wasm",
	}
}

// ArchListString returns the valid architectures as a comma-separated list.
func ArchListString() string {
	return strings.Join(Archs(), ", ")
}

// IsValidArch checks if arch is a known CPU architecture.
func IsValidArch(arch string) bool {
	for _, a := range Archs() {
		if arch == a {
			return true
		}
	}
	return false
}

// ArchSuggestion returns a suggested valid architecture for common misspellings.
func ArchSuggestion(arch string) string {
	switch arch {
	case "aarch64":
		return archARM64
	case "x86_64":
		return archAMD64
	case "amd":
		return archAMD64
	case "i386", "i686":
		return arch386
	case "armv7", "armv7l":
		return archARM
	case "armv8", "arm64v8":
		return archARM64
	}
	return ""
}
