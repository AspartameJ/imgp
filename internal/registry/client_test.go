package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"

	"gitcode.com/DonaldTom/imgp/internal/config"
)

func TestTransport_Default(t *testing.T) {
	cfg := config.DefaultConfig()
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("docker.io")
	tr := c.transport(reg)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatal("transport should be *http.Transport")
	}
	if ht.MaxConnsPerHost != 100 {
		t.Errorf("MaxConnsPerHost = %d, want 100", ht.MaxConnsPerHost)
	}
	if ht.TLSClientConfig != nil && ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false by default")
	}
}

func TestTransport_Insecure(t *testing.T) {
	cfg := config.DefaultConfig()
	c := NewClient(cfg).WithInsecure(true)
	reg, _ := name.NewRegistry("docker.io")
	tr := c.transport(reg)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatal("transport should be *http.Transport")
	}
	if ht.TLSClientConfig == nil || !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true when insecure flag is set")
	}
}

func TestTransport_InsecureRegistry(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.InsecureRegistries = []string{"reg.io"}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("reg.io")
	tr := c.transport(reg)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatal("transport should be *http.Transport")
	}
	if ht.TLSClientConfig == nil || !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true for insecure registry")
	}
}

func TestTransport_InsecureRegistrySuffix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.InsecureRegistries = []string{"reg.io"}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("sub.reg.io")
	tr := c.transport(reg)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatal("transport should be *http.Transport")
	}
	if ht.TLSClientConfig == nil || !ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true for subdomain of insecure registry")
	}
}

func TestTransport_NonInsecureRegistry(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.InsecureRegistries = []string{"reg.io"}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("other.io")
	tr := c.transport(reg)
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatal("transport should be *http.Transport")
	}
	if ht.TLSClientConfig != nil && ht.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false for non-insecure registry")
	}
}

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
	// Input validation is handled by cmd.resolvePlatform.
	// parsePlatform is lenient and returns a best-effort parse.
	_ = parsePlatform("/linux")
	_ = parsePlatform("linux//arm64")
}

func TestAuthenticator_Credentials(t *testing.T) {
	cfg := config.DefaultConfig()
	c := NewClient(cfg).WithAuth("myuser", "mypass")
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "myuser" || ac.Password != "mypass" {
		t.Errorf("got %s/%s, want myuser/mypass", ac.Username, ac.Password)
	}
}

func TestAuthenticator_ConfigAuth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auths = map[string]config.AuthConfig{
		"docker.io": {Username: "cfguser", Password: "cfgpass"},
	}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "cfguser" || ac.Password != "cfgpass" {
		t.Errorf("got %s/%s, want cfguser/cfgpass", ac.Username, ac.Password)
	}
}

func TestAuthenticator_ConfigAuth_WithPasswordEnv(t *testing.T) {
	os.Setenv("TEST_IMG_PASS", "envpass")
	defer os.Unsetenv("TEST_IMG_PASS")

	cfg := config.DefaultConfig()
	cfg.Auths = map[string]config.AuthConfig{
		"docker.io": {Username: "envuser", Password: "fallback", PasswordEnv: "TEST_IMG_PASS"},
	}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "envuser" || ac.Password != "envpass" {
		t.Errorf("got %s/%s, want envuser/envpass", ac.Username, ac.Password)
	}
}

func TestAuthenticator_ConfigAuth_PasswordEnvFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auths = map[string]config.AuthConfig{
		"docker.io": {Username: "falluser", Password: "fallpass", PasswordEnv: "UNSET_VAR"},
	}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "falluser" || ac.Password != "fallpass" {
		t.Errorf("got %s/%s, want falluser/fallpass", ac.Username, ac.Password)
	}
}

func TestAuthenticator_WildcardAuth(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auths = map[string]config.AuthConfig{
		"*": {Username: "wilduser", Password: "wildpass"},
	}
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("k8s.gcr.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "wilduser" || ac.Password != "wildpass" {
		t.Errorf("got %s/%s, want wilduser/wildpass", ac.Username, ac.Password)
	}
}

func TestAuthenticator_Anonymous(t *testing.T) {
	cfg := config.DefaultConfig()
	c := NewClient(cfg)
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	if auth != authn.Anonymous {
		t.Error("expected authn.Anonymous")
	}
}

func TestAuthenticator_CredentialsPrecedence(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Auths = map[string]config.AuthConfig{
		"docker.io": {Username: "cfguser", Password: "cfgpass"},
	}
	c := NewClient(cfg).WithAuth("cli_user", "cli_pass")
	reg, _ := name.NewRegistry("docker.io")
	auth := c.authenticator(reg)
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatalf("Authorization() error = %v", err)
	}
	if ac.Username != "cli_user" || ac.Password != "cli_pass" {
		t.Errorf("got %s/%s, want cli_user/cli_pass", ac.Username, ac.Password)
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

func TestFetchImage_RetryThenSucceed(t *testing.T) {
	var manifestAttempts int

	img, _ := random.Image(1024, 1)
	rawManifest, _ := img.RawManifest()
	m := &v1.Manifest{}
	json.Unmarshal(rawManifest, m)
	cfgBlob, _ := img.RawConfigFile()
	configDigest := sha256.Sum256(cfgBlob)
	layers, _ := img.Layers()
	ld, _ := layers[0].Digest()
	rc, _ := layers[0].Compressed()
	lData, _ := io.ReadAll(rc)
	rc.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v2/"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/manifests/"):
			manifestAttempts++
			if manifestAttempts <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", string(m.MediaType))
			w.Write(rawManifest)
		case strings.Contains(r.URL.Path, "/blobs/sha256:"+hex.EncodeToString(configDigest[:])):
			w.Write(cfgBlob)
		case strings.Contains(r.URL.Path, "/blobs/sha256:"+ld.Hex):
			w.Write(lData)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	svrPort := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/test:latest", svrPort)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	client := NewClient(cfg).WithRetry(3)

	_, _, err := client.FetchImage(context.Background(), image, "")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
}

func TestFetchImage_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	image := fmt.Sprintf("localhost:%s/slow:latest", port)

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	client := NewClient(cfg).WithRetry(0)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := client.FetchImage(ctx, image, "")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") &&
		!strings.Contains(err.Error(), "context deadline") &&
		!strings.Contains(err.Error(), "canceled") {
		t.Errorf("expected deadline/cancel error, got: %v", err)
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

func TestNewLayerFetcher(t *testing.T) {
	server, img, refStr := mockRegistry(t, 1024, 1)
	defer server.Close()

	ref, err := name.ParseReference(refStr)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	client := NewClient(cfg)

	fetcher := client.NewLayerFetcher(ref)
	if fetcher == nil {
		t.Fatal("NewLayerFetcher returned nil")
	}

	layers, _ := img.Layers()
	d, _ := layers[0].Digest()

	rc, err := fetcher(context.Background(), d.Hex)
	if err != nil {
		t.Fatalf("fetcher error: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if len(data) == 0 {
		t.Error("expected layer data")
	}
}

func TestResolveRefs_TagMirror(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MirrorMap = map[string][]string{
		"docker.io": {"invalid!!!"},
	}
	client := NewClient(cfg)
	ref, err := name.ParseReference("test:latest")
	if err != nil {
		t.Fatal(err)
	}
	refs := client.resolveRefs(ref)
	if len(refs) != 1 {
		t.Errorf("expected 1 ref (original only, bad mirror skipped), got %d", len(refs))
	}
}

func TestResolveRefs_DigestMirror(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MirrorMap = map[string][]string{
		"docker.io": {"mirror.example.com"},
	}
	client := NewClient(cfg)
	ref, err := name.NewDigest("test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	refs := client.resolveRefs(ref)
	// Should return mirror + original
	if len(refs) != 2 {
		t.Errorf("expected 2 refs (mirror + original), got %d", len(refs))
	}
}

func TestResolveRefs_DigestMirror_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MirrorMap = map[string][]string{
		"docker.io": {"invalid!!!"},
	}
	client := NewClient(cfg)
	ref, err := name.NewDigest("test@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	refs := client.resolveRefs(ref)
	// Invalid mirror should be skipped, only original returned
	if len(refs) != 1 {
		t.Errorf("expected 1 ref (bad mirror skipped), got %d", len(refs))
	}
}

func TestResolveRefs_NoMirror(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	client := NewClient(cfg)
	ref, err := name.ParseReference("test:latest")
	if err != nil {
		t.Fatal(err)
	}
	refs := client.resolveRefs(ref)
	if len(refs) != 1 {
		t.Errorf("expected 1 ref, got %d", len(refs))
	}
	if refs[0] != ref {
		t.Error("expected original ref")
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
