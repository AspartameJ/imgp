package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gitcode.com/DonaldTom/imgp/internal/config"
)

func TestCacheInfo_Empty(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	cacheDir = tmpDir
	defer func() { cacheDir = "" }()

	path, count, size := cacheInfo(cfg)
	if path != tmpDir {
		t.Errorf("path = %q, want %q", path, tmpDir)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
}

func TestCacheInfo_WithFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	cacheDir = tmpDir
	defer func() { cacheDir = "" }()

	os.WriteFile(filepath.Join(tmpDir, "abc123.gz"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(tmpDir, "def456.gz"), make([]byte, 200), 0644)
	os.WriteFile(filepath.Join(tmpDir, "other.txt"), []byte("ignore"), 0644)

	path, count, size := cacheInfo(cfg)
	if path != tmpDir {
		t.Errorf("path = %q, want %q", path, tmpDir)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if size != 300 {
		t.Errorf("size = %d, want 300", size)
	}
}

func TestCacheClear(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	cacheDir = tmpDir
	defer func() { cacheDir = "" }()

	os.WriteFile(filepath.Join(tmpDir, "abc.gz"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(tmpDir, "def.gz"), make([]byte, 200), 0644)

	removed, freed := cacheClear(cfg)
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if freed != 300 {
		t.Errorf("freed = %d, want 300", freed)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "abc.gz")); !os.IsNotExist(err) {
		t.Error("abc.gz should be removed")
	}
}

func TestCacheClear_WithVerifiedFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	cacheDir = tmpDir
	defer func() { cacheDir = "" }()

	os.WriteFile(filepath.Join(tmpDir, "abc.gz"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(tmpDir, "abc.gz.verified"), nil, 0644)
	os.WriteFile(filepath.Join(tmpDir, "def.gz"), make([]byte, 200), 0644)

	removed, freed := cacheClear(cfg)
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	if freed != 300 {
		t.Errorf("freed = %d, want 300", freed)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "abc.gz.verified")); !os.IsNotExist(err) {
		t.Error(".verified file should be removed")
	}
}

func TestCacheClear_Empty(t *testing.T) {
	cfg := config.DefaultConfig()
	tmpDir := t.TempDir()
	cacheDir = tmpDir
	defer func() { cacheDir = "" }()

	removed, freed := cacheClear(cfg)
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if freed != 0 {
		t.Errorf("freed = %d, want 0", freed)
	}
}
