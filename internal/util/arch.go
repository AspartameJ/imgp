package util

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// networkSubstrings match transport-level failures: worth retrying and, when
// a manifest fetch fails this way, worth suggesting a mirror for.
var networkSubstrings = []string{
	"unexpected EOF", "connection reset", "connection refused",
	"TLS handshake", "broken pipe", "dial tcp", "i/o timeout",
}

// contentSubstrings match content-level download failures (truncated or
// corrupted layer data). Retryable, but not a connectivity problem.
var contentSubstrings = []string{
	"incomplete download", "digest mismatch",
}

func containsAny(msg string, subs []string) bool {
	for _, s := range subs {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func httpStatusCode(msg string) int {
	// Errors may be prefixed with a URL (e.g. "GET https://...: unexpected
	// status code 502 Bad Gateway"), so locate the marker instead of parsing
	// from the start of the message.
	const marker = "unexpected status code"
	i := strings.Index(msg, marker)
	if i < 0 {
		return 0
	}
	var code int
	if _, err := fmt.Sscanf(msg[i+len(marker):], "%d", &code); err != nil {
		return 0
	}
	return code
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
	return containsAny(msg, networkSubstrings) || containsAny(msg, contentSubstrings)
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
	return containsAny(err.Error(), networkSubstrings)
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
