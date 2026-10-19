package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
)

// mockRegistryServer creates an httptest server that serves a random OCI image.
// Returns the server, the image, raw manifest bytes, and the image reference string.
func mockRegistryServer(t *testing.T, imgSize int64, numLayers int64) (*httptest.Server, v1.Image, []byte, string) {
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

	blobs := imageBlobs(t, img)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/manifests/"):
			w.Header().Set("Content-Type", string(m.MediaType))
			w.Write(rawManifest)
		case strings.Contains(r.URL.Path, "/blobs/"):
			parts := strings.Split(r.URL.Path, "/")
			digestHex := strings.TrimPrefix(parts[len(parts)-1], "sha256:")
			data, ok := blobs[digestHex]
			if !ok {
				http.Error(w, "blob not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			if rng := r.Header.Get("Range"); rng != "" {
				var start int64
				if _, err := fmt.Sscanf(rng, "bytes=%d-", &start); err == nil && start < int64(len(data)) {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
					w.WriteHeader(http.StatusPartialContent)
					w.Write(data[start:])
					return
				}
			}
			w.Write(data)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	srvURL, _ := url.Parse(srv.URL)
	refStr := fmt.Sprintf("localhost:%s/testimage:latest", srvURL.Port())
	return srv, img, rawManifest, refStr
}

// imageBlobs collects config and layer blobs from an image into a map.
func imageBlobs(t *testing.T, img v1.Image) map[string][]byte {
	t.Helper()
	blobs := make(map[string][]byte)

	configData, err := img.RawConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	cd := sha256.Sum256(configData)
	blobs[hex.EncodeToString(cd[:])] = configData

	layers, err := img.Layers()
	if err != nil {
		t.Fatal(err)
	}
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
	return blobs
}

// TestSaveGlobalsCoversAllFlags fails when a flag variable registered in
// root.go is missing from saveGlobals, which would leak state across tests.
func TestSaveGlobalsCoversAllFlags(t *testing.T) {
	rootSrc, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatal(err)
	}
	helperSrc, err := os.ReadFile("helpers_test.go")
	if err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`Var[A-Za-z]*\(&([A-Za-z0-9_]+)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(rootSrc), -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !strings.Contains(string(helperSrc), name+" = ") {
			t.Errorf("flag %q is registered in root.go but not restored by saveGlobals", name)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no flag variables found in root.go; regexp or file layout changed")
	}
}

// saveGlobals saves all package-level test flags into a snapshot function.
// Keep the snapshot in sync with the flag variables declared in root.go:
// adding a global flag without listing it here leaks state across tests.
// TestSaveGlobalsCoversAllFlags guards against that.
func saveGlobals() func() {
	p := platform
	o := output
	q := quiet
	g := gzip
	n := noCache
	c := cacheDir
	u := username
	i := insecure
	pl := parallelism
	r := retryCount
	t := timeoutMin
	lt := layerTimeoutMin
	pw := password
	pe := passwordEnv
	nr := noResume
	return func() {
		platform = p
		output = o
		quiet = q
		gzip = g
		noCache = n
		cacheDir = c
		username = u
		insecure = i
		parallelism = pl
		retryCount = r
		timeoutMin = t
		layerTimeoutMin = lt
		password = pw
		passwordEnv = pe
		noResume = nr
	}
}
