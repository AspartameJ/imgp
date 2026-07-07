package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Parallelism != DefaultParallelism {
		t.Errorf("Parallelism = %d, want %d", cfg.Parallelism, DefaultParallelism)
	}
	if cfg.MirrorMap == nil {
		t.Error("MirrorMap should not be nil")
	}
	if cfg.Retry != 2 {
		t.Errorf("Retry = %d, want 2", cfg.Retry)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Parallelism != DefaultParallelism {
		t.Errorf("Parallelism = %d, want %d", cfg.Parallelism, DefaultParallelism)
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")
	t.Logf("config path: %s", cp)

	data := `{"parallelism": 8, "retry": 5, "mirror_map": {"docker.io": ["m.test"]}}`
	if err := os.WriteFile(cp, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	orig := ConfigPath
	ConfigPath = func() string { return cp }
	defer func() { ConfigPath = orig }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Parallelism != 8 {
		t.Errorf("Parallelism = %d, want 8", cfg.Parallelism)
	}
	if cfg.Retry != 5 {
		t.Errorf("Retry = %d, want 5", cfg.Retry)
	}
	if cfg.MirrorMap["docker.io"] == nil || cfg.MirrorMap["docker.io"][0] != "m.test" {
		t.Errorf("MirrorMap docker.io = %v, want [m.test]", cfg.MirrorMap["docker.io"])
	}
}

func TestSave(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")
	orig := ConfigPath
	ConfigPath = func() string { return cp }
	defer func() { ConfigPath = orig }()

	cfg := DefaultConfig()
	cfg.configPath = cp
	cfg.Parallelism = 6
	cfg.CacheDir = "/test/cache"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg2.Parallelism != 6 {
		t.Errorf("Parallelism = %d, want 6", cfg2.Parallelism)
	}
	if cfg2.CacheDir != "/test/cache" {
		t.Errorf("CacheDir = %q, want /test/cache", cfg2.CacheDir)
	}
}

func TestSave_StripPassword(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")
	orig := ConfigPath
	ConfigPath = func() string { return cp }
	defer func() { ConfigPath = orig }()

	cfg := DefaultConfig()
	cfg.configPath = cp
	cfg.Auths = map[string]AuthConfig{
		"reg.io": {Username: "u", Password: "secret", PasswordEnv: "PASS"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	a := cfg2.Auths["reg.io"]
	if a.Password != "" {
		t.Error("Password should not be persisted")
	}
	if a.Username != "u" {
		t.Errorf("Username = %q, want u", a.Username)
	}
	if a.PasswordEnv != "PASS" {
		t.Errorf("PasswordEnv = %q, want PASS", a.PasswordEnv)
	}
}

func TestEffectiveCacheDir(t *testing.T) {
	cfg := &Config{CacheDir: "/custom/path"}
	if got := cfg.EffectiveCacheDir(); got != "/custom/path" {
		t.Errorf("EffectiveCacheDir = %q, want /custom/path", got)
	}

	cfg2 := &Config{}
	got := cfg2.EffectiveCacheDir()
	if got == "" {
		t.Error("EffectiveCacheDir should return non-empty default")
	}
}
