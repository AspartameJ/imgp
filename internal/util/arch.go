package util

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

var retryableSubstrings = []string{
	"unexpected EOF", "connection reset", "connection refused",
	"TLS handshake", "broken pipe", "dial tcp", "i/o timeout",
}

func containsRetryable(msg string) bool {
	for _, s := range retryableSubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func httpStatusCode(msg string) int {
	var code int
	if _, err := fmt.Sscanf(msg, "unexpected status code %d", &code); err == nil {
		return code
	}
	return 0
}

func isNetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// IsRetryable determines whether an error should trigger a retry.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if isNetError(err) {
		return true
	}
	msg := err.Error()
	if code := httpStatusCode(msg); code != 0 {
		return code >= 500
	}
	return containsRetryable(msg)
}

// IsConnectivityError returns true for errors that indicate a network connectivity
// problem (as opposed to a server-side error like 5xx). This is useful for
// deciding whether to suggest using a mirror to the user.
func IsConnectivityError(err error) bool {
	if err == nil {
		return false
	}
	if isNetError(err) {
		return true
	}
	return containsRetryable(err.Error())
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
