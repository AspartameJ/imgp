package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type AuthConfig struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	PasswordEnv string `json:"password_env,omitempty"`
}

type Config struct {
	MirrorMap          map[string][]string   `json:"mirror_map"`
	Auths              map[string]AuthConfig `json:"auths,omitempty"`
	InsecureRegistries []string              `json:"insecure_registries,omitempty"`
	Parallelism        int                   `json:"parallelism"`
	LayerTimeout       int                   `json:"layer_timeout,omitempty"`
	Timeout            int                   `json:"timeout,omitempty"`
	Retry              int                   `json:"retry,omitempty"`

	configPath string
}

func DefaultConfig() *Config {
	return &Config{
		MirrorMap: map[string][]string{
			"docker.io":       {"docker.m.daocloud.io"},
			"gcr.io":          {"gcr.mirrors.daocloud.io"},
			"registry.k8s.io": {"m.daocloud.io/registry.k8s.io"},
		},
		Parallelism: 4,
		Retry:       2,
	}
}

func ConfigPath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "imgp.json")
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "imgp.json")
	}
	return filepath.Join(".", "imgp.json")
}

func Load() (*Config, error) {
	cfg := DefaultConfig()
	cfg.configPath = ConfigPath()

	data, err := os.ReadFile(cfg.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Parallelism < 1 {
		cfg.Parallelism = 4
	}
	if cfg.LayerTimeout < 0 {
		cfg.LayerTimeout = 0
	}
	if cfg.Timeout < 0 {
		cfg.Timeout = 0
	}
	if cfg.Retry < 0 {
		cfg.Retry = 0
	}

	return cfg, nil
}

func (c *Config) Save() error {
	// Password fields are intentionally not persisted for security.
	// Use PasswordEnv to reference a secure environment variable instead.
	saveCfg := *c
	saveCfg.Auths = make(map[string]AuthConfig, len(c.Auths))
	for k, v := range c.Auths {
		saveCfg.Auths[k] = AuthConfig{
			Username:    v.Username,
			PasswordEnv: v.PasswordEnv,
		}
	}
	data, err := json.MarshalIndent(saveCfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0600)
}

func CacheDir() string {
	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			return filepath.Join(localAppData, "imgp", "cache")
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, "Library", "Caches", "imgp")
		}
	default: // linux and others
		if cacheHome := os.Getenv("XDG_CACHE_HOME"); cacheHome != "" {
			return filepath.Join(cacheHome, "imgp")
		}
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, ".cache", "imgp")
		}
	}
	// Fallback
	return filepath.Join(os.TempDir(), "imgp-cache")
}
