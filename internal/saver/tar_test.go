package saver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
)

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

func cacheFn(dir string) func(string) string {
	return func(digest string) string {
		return filepath.Join(dir, "cache", digest+".gz")
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
		cacheDir := filepath.Join(dir, "cache")
		os.MkdirAll(cacheDir, 0755)
		cacheFile := filepath.Join(cacheDir, digest.Hex+".gz")
		if err := os.WriteFile(cacheFile, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
