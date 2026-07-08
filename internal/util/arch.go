package util

import (
	"errors"
	"net"
	"strings"
)

// IsRetryable determines whether an error should trigger a retry.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "unexpected status code 4") {
		return false
	}
	retryable := []string{"unexpected EOF", "connection reset", "connection refused",
		"TLS handshake", "broken pipe"}
	for _, s := range retryable {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return strings.Contains(msg, "unexpected status code 5")
}

// IsValidArch checks if arch is a known CPU architecture.
func IsValidArch(arch string) bool {
	switch arch {
	case "386", "amd64", "arm", "arm64", "loong64", "mips",
		"mips64", "mips64le", "mipsle", "ppc64", "ppc64le",
		"riscv64", "s390x", "wasm":
		return true
	}
	return false
}

// ArchSuggestion returns a suggested valid architecture for common misspellings.
func ArchSuggestion(arch string) string {
	switch arch {
	case "aarch64":
		return "arm64"
	case "x86_64":
		return "amd64"
	case "amd":
		return "amd64"
	case "i386", "i686":
		return "386"
	case "armv7", "armv7l":
		return "arm"
	case "armv8", "arm64v8":
		return "arm64"
	}
	return ""
}
