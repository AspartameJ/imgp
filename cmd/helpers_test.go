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

// saveGlobals saves all package-level test flags into a snapshot function.
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
	}
}
