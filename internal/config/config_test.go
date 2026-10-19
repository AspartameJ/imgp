package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOsDefaultCacheDir_XDG(t *testing.T) {
	const laKey = "LOCALAPPDATA"
	const xdgKey = "XDG_CACHE_HOME"
	oldLA := os.Getenv(laKey)
	oldXDG := os.Getenv(xdgKey)
	defer func() {
		os.Setenv(laKey, oldLA)
		os.Setenv(xdgKey, oldXDG)
	}()
	os.Unsetenv(laKey)
	os.Setenv(xdgKey, "/custom/xdg")

	dir := osDefaultCacheDir()
	want := filepath.Join("/custom/xdg", "imgp")
	if dir != want {
		t.Errorf("osDefaultCacheDir() = %q, want %q", dir, want)
	}
}

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

	data := `{"parallelism": 8, "retry": 5, "mirror_map": {"docker.io": ["m.test"]}}`
	if err := os.WriteFile(cp, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cp)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
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

	cfg := DefaultConfig()
	cfg.configPath = cp
	cfg.Parallelism = 6
	cfg.CacheDir = "/test/cache"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg2, err := LoadFrom(cp)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
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

	cfg := DefaultConfig()
	cfg.configPath = cp
	cfg.Auths = map[string]AuthConfig{
		"reg.io": {Username: "u", Password: "secret", PasswordEnv: "PASS"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadFrom(cp)
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

func TestSave_RetryZeroRoundTrip(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")

	cfg := DefaultConfig()
	cfg.configPath = cp
	cfg.Retry = 0
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg2, err := LoadFrom(cp)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg2.Retry != 0 {
		t.Errorf("Retry = %d, want 0 (retry must survive omitempty-less save)", cfg2.Retry)
	}
}

func TestOsDefaultCacheDir(t *testing.T) {
	t.Run("with LOCALAPPDATA", func(t *testing.T) {
		original := os.Getenv("LOCALAPPDATA")
		defer os.Setenv("LOCALAPPDATA", original)
		os.Setenv("LOCALAPPDATA", "D:\\test\\appdata")

		got := osDefaultCacheDir()
		want := "D:\\test\\appdata\\imgp\\cache"
		if got != want {
			t.Errorf("osDefaultCacheDir = %q, want %q", got, want)
		}
	})

	t.Run("LOCALAPPDATA unset falls back to TempDir", func(t *testing.T) {
		original := os.Getenv("LOCALAPPDATA")
		defer os.Setenv("LOCALAPPDATA", original)
		os.Unsetenv("LOCALAPPDATA")

		got := osDefaultCacheDir()
		if got == "" {
			t.Error("osDefaultCacheDir should not be empty")
		}
	})
}

func TestLoadFrom_InvalidJSON(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")
	if err := os.WriteFile(cp, []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFrom(cp)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFrom_ReadError(t *testing.T) {
	// os.ReadFile on a directory returns a non-NotExist error.
	tmp := t.TempDir()
	_, err := LoadFrom(tmp)
	if err == nil {
		t.Fatal("expected error for reading a directory as config")
	}
}

func TestLoadFrom_Validation(t *testing.T) {
	t.Run("nil MirrorMap gets default", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"mirror_map": null}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MirrorMap == nil {
			t.Error("MirrorMap should not be nil after validation")
		}
	})

	t.Run("Parallelism less than 1 defaults", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"parallelism": 0}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Parallelism != DefaultParallelism {
			t.Errorf("Parallelism = %d, want %d", cfg.Parallelism, DefaultParallelism)
		}
	})

	t.Run("negative LayerTimeout clamps to 0", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"layer_timeout": -5}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LayerTimeout == nil || *cfg.LayerTimeout != 0 {
			t.Errorf("LayerTimeout = %v, want 0", cfg.LayerTimeout)
		}
	})

	t.Run("negative Timeout clamps to 0", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"timeout": -1}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout == nil || *cfg.Timeout != 0 {
			t.Errorf("Timeout = %v, want 0", cfg.Timeout)
		}
	})

	t.Run("negative Retry clamps to 0", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"retry": -3}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Retry != 0 {
			t.Errorf("Retry = %d, want 0", cfg.Retry)
		}
	})
}

func TestLoadFrom_NormalizesRegistryKeys(t *testing.T) {
	cp := filepath.Join(t.TempDir(), "imgp.json")
	data := `{
		"mirror_map": {"index.docker.io": ["m.test"], "quay.io:443": ["q.test"]},
		"auths": {"registry-1.docker.io": {"username": "u"}, "*": {"username": "w"}}
	}`
	if err := os.WriteFile(cp, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(cp)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.MirrorMap["docker.io"]; len(got) != 1 || got[0] != "m.test" {
		t.Errorf(`MirrorMap["docker.io"] = %v, want [m.test] (alias key normalized)`, got)
	}
	if got := cfg.MirrorMap["quay.io"]; len(got) != 1 || got[0] != "q.test" {
		t.Errorf(`MirrorMap["quay.io"] = %v, want [q.test] (:443 stripped)`, got)
	}
	if _, ok := cfg.Auths["docker.io"]; !ok {
		t.Errorf("Auths[docker.io] missing, aliases must be normalized: %v", cfg.Auths)
	}
	if _, ok := cfg.Auths["*"]; !ok {
		t.Error(`wildcard "*" auth must be preserved`)
	}
}

func TestLoadFrom_MirrorMapMergeAndClear(t *testing.T) {
	t.Run("built-ins kept, custom wins", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"mirror_map": {"docker.io": ["m.test"]}}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.MirrorMap["docker.io"]; len(got) != 1 || got[0] != "m.test" {
			t.Errorf(`MirrorMap["docker.io"] = %v, want [m.test]`, got)
		}
		if _, ok := cfg.MirrorMap["gcr.io"]; !ok {
			t.Error("built-in mirrors should remain when only one entry is overridden")
		}
	})

	t.Run("explicit empty disables all mirrors", func(t *testing.T) {
		cp := filepath.Join(t.TempDir(), "imgp.json")
		if err := os.WriteFile(cp, []byte(`{"mirror_map": {}}`), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadFrom(cp)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.MirrorMap) != 0 {
			t.Errorf("MirrorMap = %v, want empty (explicit clear must not re-add defaults)", cfg.MirrorMap)
		}
	})
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
