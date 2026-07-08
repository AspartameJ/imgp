package cmd

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCacheInfoCmd_NotExist(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "imgp-test-cmd-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()

	saveCacheDir := cacheDir
	defer func() { cacheDir = saveCacheDir }()
	cacheDir = filepath.Join(baseDir, "nonexistent")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheInfoCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	if !strings.Contains(string(out), "not yet created") {
		t.Errorf("expected 'not yet created', got: %s", string(out))
	}
}

func TestCacheClearCmd_Empty(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "imgp-test-clear-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()

	saveCacheDir := cacheDir
	defer func() { cacheDir = saveCacheDir }()
	cacheDir = baseDir

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheClearCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	if !strings.Contains(string(out), "Cleared 0") {
		t.Errorf("expected 'Cleared 0', got: %s", string(out))
	}
}

func TestCacheClearCmd_WithFiles(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "imgp-test-clear2-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()
	os.WriteFile(filepath.Join(baseDir, "abc.gz"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(baseDir, "def.gz"), make([]byte, 200), 0644)

	saveCacheDir := cacheDir
	defer func() { cacheDir = saveCacheDir }()
	cacheDir = baseDir

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cacheClearCmd.RunE(nil, nil)

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()

	s := string(out)
	if !strings.Contains(s, "Cleared 2") {
		t.Errorf("expected 'Cleared 2', got: %s", s)
	}
	if !strings.Contains(s, "300 B") {
		t.Errorf("expected '300 B', got: %s", s)
	}
}
