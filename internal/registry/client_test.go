package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"

	"gitcode.com/DonaldTom/imgp/internal/config"
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

// mockRegistry creates an httptest server that serves a random image.
// Returns the server, the raw manifest bytes, and the image reference string.
func mockRegistry(t *testing.T, imgSize int64, numLayers int64) (*httptest.Server, v1.Image, string) {
	t.Helper()
	img, err := random.Image(imgSize, numLayers)
	if err != nil {
		t.Fatal(err)
	}

	rawManifest, err := img.RawManifest()
	if err != nil {
		t.Fatal(err)
	}

	m := &v1.Manifest{}
	if err := json.Unmarshal(rawManifest, m); err != nil {
		t.Fatal(err)
	}

	// Read layer data into memory for serving
	layers, err := img.Layers()
	if err != nil {
		t.Fatal(err)
	}

	blobs := make(map[string][]byte)

	// Config blob
	config, err := img.RawConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	configDigest := sha256.Sum256(config)
	blobs[hex.EncodeToString(configDigest[:])] = config

	// Layer blobs
	for _, l := range layers {
		d, err := l.Digest()
		if err != nil {
			t.Fatal(err)
		}
		rc, err := l.Compressed()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		blobs[d.Hex] = data
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/v2/":
			w.WriteHeader(http.StatusOK)

		case strings.Contains(path, "/manifests/"):
			w.Header().Set("Content-Type", string(m.MediaType))
			w.Write(rawManifest)

		case strings.Contains(path, "/blobs/"):
			parts := strings.Split(path, "/")
			digestHex := strings.TrimPrefix(parts[len(parts)-1], "sha256:")
			data, ok := blobs[digestHex]
			if !ok {
				http.Error(w, "blob not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	refStr := fmt.Sprintf("localhost:%s/testimage:latest", port)

	return server, img, refStr
}

func TestFetchImage_Success(t *testing.T) {
	server, origImg, refStr := mockRegistry(t, 1024, 2)
	defer server.Close()

	cfg := config.DefaultConfig()
	// Clear MirrorMap so we don't try external mirrors
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithRetry(0)

	img, ref, err := client.FetchImage(context.Background(), refStr, "linux/amd64")
	if err != nil {
		t.Fatalf("FetchImage() error = %v", err)
	}
	if img == nil {
		t.Fatal("img is nil")
	}
	if ref == nil {
		t.Fatal("ref is nil")
	}

	origDigest, _ := origImg.Digest()
	gotDigest, _ := img.Digest()
	if origDigest != gotDigest {
		t.Errorf("digest mismatch: %v vs %v", origDigest, gotDigest)
	}
}

func TestFetchImage_NotFound(t *testing.T) {
	// Server that only responds to /v2/ but not to any manifests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/nonexistent:latest", port)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithRetry(0)

	_, _, err := client.FetchImage(context.Background(), image, "")
	if err == nil {
		t.Fatal("expected error for nonexistent image, got nil")
	}
}

func TestFetchImage_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/test:latest", port)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithRetry(0)

	_, _, err := client.FetchImage(context.Background(), image, "")
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got: %v", err)
	}
}

func TestFetchImage_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/test:latest", port)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithRetry(1)

	_, _, err := client.FetchImage(context.Background(), image, "")
	if err == nil {
		t.Fatal("expected error for persistent 503, got nil")
	}
}

func TestFetchImage_MirrorFallback(t *testing.T) {
	// Primary server returns 404
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer primary.Close()

	// Mirror server serves the image
	mirror, origImg, _ := mockRegistry(t, 512, 1)
	defer mirror.Close()

	mirrorHost := strings.TrimPrefix(mirror.URL, "http://")
	image := strings.TrimPrefix(primary.URL, "http://") + "/myimage:latest"

	// MirrorMap: primary host -> mirror host
	primaryHost := strings.SplitN(image, "/", 2)[0]
	cfg := config.DefaultConfig()
	cfg.MirrorMap = map[string][]string{
		primaryHost: {mirrorHost},
	}

	client := NewClient(cfg).WithRetry(0)

	img, ref, err := client.FetchImage(context.Background(), image, "")
	if err != nil {
		t.Fatalf("FetchImage() error = %v", err)
	}
	if img == nil {
		t.Fatal("img is nil")
	}
	if ref == nil {
		t.Fatal("ref is nil")
	}

	origDigest, _ := origImg.Digest()
	gotDigest, _ := img.Digest()
	if origDigest != gotDigest {
		t.Errorf("digest mismatch: %v vs %v", origDigest, gotDigest)
	}
}

func TestWithAuth(t *testing.T) {
	var gotUser, gotPass string
	manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":100},"layers":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if ok {
			gotUser, gotPass = u, p
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Write([]byte(manifest))
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/test:latest", port)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithAuth("myuser", "mypass").WithRetry(0)

	img, _, err := client.FetchImage(context.Background(), image, "")
	if err != nil {
		t.Fatalf("FetchImage() error = %v (expected success, auth was sent)", err)
	}
	if gotUser != "myuser" || gotPass != "mypass" {
		t.Errorf("auth: got %s/%s, want myuser/mypass", gotUser, gotPass)
	}
	_, err = img.RawConfigFile()
	if err == nil {
		t.Fatal("expected error fetching missing config blob")
	}
}
