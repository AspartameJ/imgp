package saver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type errLayer struct{ err error }

func (e *errLayer) Digest() (v1.Hash, error)             { return v1.Hash{}, e.err }
func (e *errLayer) DiffID() (v1.Hash, error)             { return v1.Hash{}, e.err }
func (e *errLayer) Compressed() (io.ReadCloser, error)   { return nil, e.err }
func (e *errLayer) Uncompressed() (io.ReadCloser, error) { return nil, e.err }
func (e *errLayer) Size() (int64, error)                 { return 0, e.err }
func (e *errLayer) MediaType() (types.MediaType, error)  { return "", e.err }

func TestOpenCachedLayer_DigestError(t *testing.T) {
	_, err := OpenCachedLayer(&errLayer{err: io.ErrUnexpectedEOF}, "dummy.gz")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenCachedLayer_MediaTypeError(t *testing.T) {
	// Provide a layer that returns digest OK but fails on MediaType
	l := &partialDigestLayer{digest: v1.Hash{Algorithm: "sha256", Hex: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	_, err := OpenCachedLayer(l, "dummy.gz")
	if err == nil {
		t.Fatal("expected error")
	}
}

// partialDigestLayer returns digest OK but fails on other methods
type partialDigestLayer struct {
	digest v1.Hash
}

func (l *partialDigestLayer) Digest() (v1.Hash, error)             { return l.digest, nil }
func (l *partialDigestLayer) DiffID() (v1.Hash, error)             { return v1.Hash{}, io.ErrUnexpectedEOF }
func (l *partialDigestLayer) Compressed() (io.ReadCloser, error)   { return nil, io.ErrUnexpectedEOF }
func (l *partialDigestLayer) Uncompressed() (io.ReadCloser, error) { return nil, io.ErrUnexpectedEOF }
func (l *partialDigestLayer) Size() (int64, error)                 { return 0, io.ErrUnexpectedEOF }
func (l *partialDigestLayer) MediaType() (types.MediaType, error)  { return "", io.ErrUnexpectedEOF }

type errImage struct {
	err error
}

func (e *errImage) Layers() ([]v1.Layer, error)             { return nil, e.err }
func (e *errImage) MediaType() (types.MediaType, error)     { return "", e.err }
func (e *errImage) Size() (int64, error)                    { return 0, e.err }
func (e *errImage) ConfigFile() (*v1.ConfigFile, error)     { return nil, e.err }
func (e *errImage) ConfigName() (v1.Hash, error)            { return v1.Hash{}, e.err }
func (e *errImage) RawConfigFile() ([]byte, error)          { return nil, e.err }
func (e *errImage) Digest() (v1.Hash, error)                { return v1.Hash{}, e.err }
func (e *errImage) Manifest() (*v1.Manifest, error)         { return nil, e.err }
func (e *errImage) RawManifest() ([]byte, error)            { return nil, e.err }
func (e *errImage) LayerByDigest(v1.Hash) (v1.Layer, error) { return nil, e.err }
func (e *errImage) LayerByDiffID(v1.Hash) (v1.Layer, error) { return nil, e.err }

func TestBuildCachedImage_LayersError(t *testing.T) {
	_, err := buildCachedImage(&errImage{err: io.ErrUnexpectedEOF}, nopCacheFn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildCachedImage_ConfigFileError(t *testing.T) {
	img, _ := random.Image(256, 1)
	// Create an image that passes Layers() but fails on RawConfigFile()
	mock := &mockFailImage{inner: img, failOn: "rawconfig"}
	_, err := buildCachedImage(mock, nopCacheFn)
	if err == nil {
		t.Fatal("expected error")
	}
}

type mockFailImage struct {
	inner  v1.Image
	failOn string
}

func (m *mockFailImage) Layers() ([]v1.Layer, error) {
	if m.failOn == "layers" {
		return nil, io.ErrUnexpectedEOF
	}
	return m.inner.Layers()
}
func (m *mockFailImage) MediaType() (types.MediaType, error) { return m.inner.MediaType() }
func (m *mockFailImage) Size() (int64, error)                { return m.inner.Size() }
func (m *mockFailImage) ConfigFile() (*v1.ConfigFile, error) { return m.inner.ConfigFile() }
func (m *mockFailImage) ConfigName() (v1.Hash, error)        { return m.inner.ConfigName() }
func (m *mockFailImage) RawConfigFile() ([]byte, error) {
	if m.failOn == "rawconfig" {
		return nil, io.ErrUnexpectedEOF
	}
	return m.inner.RawConfigFile()
}
func (m *mockFailImage) Digest() (v1.Hash, error) { return m.inner.Digest() }
func (m *mockFailImage) Manifest() (*v1.Manifest, error) {
	if m.failOn == "manifest" {
		return nil, io.ErrUnexpectedEOF
	}
	return m.inner.Manifest()
}
func (m *mockFailImage) RawManifest() ([]byte, error) {
	if m.failOn == "rawmanifest" {
		return nil, io.ErrUnexpectedEOF
	}
	return m.inner.RawManifest()
}
func (m *mockFailImage) LayerByDigest(h v1.Hash) (v1.Layer, error) { return m.inner.LayerByDigest(h) }
func (m *mockFailImage) LayerByDiffID(h v1.Hash) (v1.Layer, error) { return m.inner.LayerByDiffID(h) }

func TestBuildCachedImage_RawManifestError(t *testing.T) {
	img, _ := random.Image(256, 1)
	mock := &mockFailImage{inner: img, failOn: "rawmanifest"}
	_, err := buildCachedImage(mock, nopCacheFn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildCachedImage_ManifestError(t *testing.T) {
	img, _ := random.Image(256, 1)
	mock := &mockFailImage{inner: img, failOn: "manifest"}
	_, err := buildCachedImage(mock, nopCacheFn)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExport_DirOutput(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)
	dir := t.TempDir()

	err := Export(ctx, ref, img, dir, nopCacheFn, false, nopProgress)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got %v", err)
	}
}

func TestExport_Basic(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 2)

	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar")

	populateCache(t, dir, img)

	err := Export(ctx, ref, img, outPath, cacheFn(dir), false, nopProgress)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tr := tar.NewReader(f)
	foundManifest := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		if hdr.Name == "manifest.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Error("tar missing manifest.json")
	}
}

func TestExport_Gzip(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)

	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar.gz")

	populateCache(t, dir, img)

	err := Export(ctx, ref, img, outPath, cacheFn(dir), true, nopProgress)
	if err != nil {
		t.Fatalf("Export(gzip=true) error = %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip.NewReader error = %v (not a valid gzip file)", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	foundManifest := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		if hdr.Name == "manifest.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Error("gzip tar missing manifest.json")
	}
}

func TestExport_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 2)

	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar")

	populateCache(t, dir, img)
	cancel()

	err := Export(ctx, ref, img, outPath, cacheFn(dir), false, nopProgress)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestExport_OverwriteExisting(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)

	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar")

	populateCache(t, dir, img)

	// Create an existing file to overwrite
	os.WriteFile(outPath, []byte("old data"), 0644)

	err := Export(ctx, ref, img, outPath, cacheFn(dir), false, nopProgress)
	if err != nil {
		t.Fatalf("Export() error = %v (should overwrite existing file)", err)
	}

	data, _ := os.ReadFile(outPath)
	if string(data) == "old data" {
		t.Error("file was not overwritten")
	}
}

func TestExport_ProgressFnPanic(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)

	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar")

	populateCache(t, dir, img)

	panicFn := func(completed, total int64) {
		panic("test panic in progress fn")
	}

	err := Export(ctx, ref, img, outPath, cacheFn(dir), false, panicFn)
	if err != nil {
		t.Fatalf("Export() error after progressFn panic = %v", err)
	}
}

func mkTempDir(t *testing.T) (string, func()) {
	dir, err := os.MkdirTemp("", "imgp-test-*")
	if err != nil {
		t.Fatal(err)
	}
	return dir, func() {
		runtime.GC()
		os.RemoveAll(dir)
	}
}

func TestCancelWriter(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cw := &cancelWriter{f: f, ctx: ctx}

	n, err := cw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned %d, want 5", n)
	}

	cancel()
	_, err = cw.Write([]byte("world"))
	if err == nil {
		t.Error("expected error after context cancellation")
	}
}

func TestOpenCachedLayer_Valid(t *testing.T) {
	img, _ := random.Image(1024, 1)
	layers, _ := img.Layers()
	l := layers[0]
	dir := t.TempDir()

	digest, _ := l.Digest()
	rc, _ := l.Compressed()
	data, _ := io.ReadAll(rc)
	rc.Close()
	cacheFile := filepath.Join(dir, digest.Hex+".gz")
	os.WriteFile(cacheFile, data, 0644)

	cl, err := OpenCachedLayer(l, cacheFile)
	if err != nil {
		t.Fatalf("OpenCachedLayer error = %v", err)
	}
	d, _ := cl.Digest()
	if d != digest {
		t.Errorf("digest = %v, want %v", d, digest)
	}
}

func TestOpenCachedLayer_Missing(t *testing.T) {
	img, _ := random.Image(1024, 1)
	layers, _ := img.Layers()
	l := layers[0]
	dir := t.TempDir()

	_, err := OpenCachedLayer(l, filepath.Join(dir, "nonexistent.gz"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestOpenCachedLayer_WrongSize(t *testing.T) {
	img, _ := random.Image(1024, 1)
	layers, _ := img.Layers()
	l := layers[0]
	dir := t.TempDir()

	digest, _ := l.Digest()
	cacheFile := filepath.Join(dir, digest.Hex+".gz")
	os.WriteFile(cacheFile, make([]byte, 10), 0644)

	_, err := OpenCachedLayer(l, cacheFile)
	if err == nil {
		t.Fatal("expected error for wrong size")
	}
}

func TestOpenCachedLayer_CorruptedGzip(t *testing.T) {
	img, _ := random.Image(1024, 1)
	layers, _ := img.Layers()
	l := layers[0]
	dir := t.TempDir()

	digest, _ := l.Digest()
	cacheFile := filepath.Join(dir, digest.Hex+".gz")
	os.WriteFile(cacheFile, []byte("not-gzip-data"), 0644)

	_, err := OpenCachedLayer(l, cacheFile)
	if err == nil {
		t.Fatal("expected error for corrupted gzip")
	}
}

func TestVerifyGzip_MarkerSkip(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "test.gz")
	os.WriteFile(cacheFile, []byte("not-valid-gzip"), 0644)
	os.WriteFile(cacheFile+".verified", nil, 0644)

	err := verifyGzip(cacheFile)
	if err != nil {
		t.Fatalf("expected skip (nil), got %v", err)
	}
}

func TestVerifyGzip_StaleMarkerRevalidates(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "test.gz")
	markerFile := cacheFile + ".verified"

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("valid data"))
	gw.Close()
	os.WriteFile(cacheFile, buf.Bytes(), 0644)
	os.WriteFile(markerFile, nil, 0644)
	os.Chtimes(markerFile, time.Now(), time.Now().Add(-time.Second))

	os.WriteFile(cacheFile, []byte("corrupted"), 0644)

	err := verifyGzip(cacheFile)
	if err == nil {
		t.Fatal("expected re-validation and error for corrupted file after marker")
	}
}

func TestVerifyGzip_CRCFailure(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "test.gz")
	// Write minimal bytes that pass gzip magic check but fail CRC check
	f, _ := os.Create(cacheFile)
	f.Write([]byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff})
	f.Close()

	err := verifyGzip(cacheFile)
	if err == nil {
		t.Fatal("expected CRC mismatch error")
	}
}

func TestCleanupTmp_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "test.tmp")
	os.WriteFile(tmpFile, []byte("x"), 0644)
	cleanupTmp(tmpFile)
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestWriteTarBall_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.tar")

	cancel()

	err := writeTarBall(ctx, ref, img, outPath, false, nopProgress)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestWriteTarBall_InvalidDir(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(1024, 1)

	// Use a path containing a NUL byte (Windows) or invalid char to trigger MkdirAll error
	err := writeTarBall(ctx, ref, img, "\x00/out.tar", false, nopProgress)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestExport_NilProgress(t *testing.T) {
	ctx := context.Background()
	ref, _ := name.ParseReference("test:latest")
	img, _ := random.Image(64, 1)
	dir, cleanup := mkTempDir(t)
	defer cleanup()
	outPath := filepath.Join(dir, "out.tar")

	populateCache(t, dir, img)

	err := Export(ctx, ref, img, outPath, cacheFn(dir), false, nil)
	if err != nil {
		t.Fatalf("Export() with nil progressFn error = %v", err)
	}
}

func TestLayerByDigest_NotFound(t *testing.T) {
	ci := &compressedImage{layers: make(map[v1.Hash]partial.CompressedLayer)}
	_, err := ci.LayerByDigest(v1.Hash{})
	if err == nil {
		t.Fatal("expected error for missing layer")
	}
}

func TestFileLayer_MediaType(t *testing.T) {
	fl := &fileLayer{mediaType: "test/type"}
	mt, err := fl.MediaType()
	if err != nil {
		t.Fatal(err)
	}
	if mt != "test/type" {
		t.Errorf("got %q", mt)
	}
}

func TestCompressedImage_MediaType(t *testing.T) {
	ci := &compressedImage{mediaType: "test/type"}
	mt, err := ci.MediaType()
	if err != nil {
		t.Fatal(err)
	}
	if mt != "test/type" {
		t.Errorf("got %q", mt)
	}
}

func TestFileLayer_Compressed(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.gz")
	os.WriteFile(filePath, []byte("data"), 0644)

	fl := &fileLayer{filepath: filePath}
	rc, err := fl.Compressed()
	if err != nil {
		t.Fatalf("Compressed() error = %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "data" {
		t.Errorf("got %q, want %q", string(data), "data")
	}
}

func cacheFn(dir string) func(string) string {
	return func(digest string) string {
		return filepath.Join(dir, digest+".gz")
	}
}

func nopCacheFn(string) string { return "" }

func nopProgress(int64, int64) {}

func populateCache(t *testing.T, dir string, img v1.Image) {
	t.Helper()
	layers, err := img.Layers()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range layers {
		digest, err := l.Digest()
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
		cacheFile := filepath.Join(dir, digest.Hex+".gz")
		if err := os.WriteFile(cacheFile, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
