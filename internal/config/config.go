package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultParallelism = 4

// AuthConfig holds registry authentication credentials.
type AuthConfig struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	PasswordEnv string `json:"password_env,omitempty"`
}

// Config holds all imgp configuration loaded from imgp.json.
type Config struct {
	MirrorMap          map[string][]string   `json:"mirror_map"`
	Auths              map[string]AuthConfig `json:"auths,omitempty"`
	InsecureRegistries []string              `json:"insecure_registries,omitempty"`
	Parallelism        int                   `json:"parallelism"`
	LayerTimeout       int                   `json:"layer_timeout,omitempty"`
	Timeout            int                   `json:"timeout,omitempty"`
	Retry              int                   `json:"retry,omitempty"`
	CacheDir           string                `json:"cache_dir,omitempty"`

	configPath string
}

// DefaultConfig returns a Config with default mirror map and parallelism.
func DefaultConfig() *Config {
	return &Config{
		MirrorMap: map[string][]string{
			"docker.io":       {"docker.m.daocloud.io"},
			"gcr.io":          {"gcr.mirrors.daocloud.io"},
			"registry.k8s.io": {"m.daocloud.io/registry.k8s.io"},
			"quay.io":         {"quay.nju.edu.cn"},
		},
		Parallelism: DefaultParallelism,
		Retry:       2,
	}
}

// ConfigPath returns the path to imgp.json (next to the binary).
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

// LoadFrom reads and parses a config from the given path.
func LoadFrom(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.configPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.MirrorMap == nil {
		cfg.MirrorMap = DefaultConfig().MirrorMap
	}
	if cfg.Parallelism < 1 {
		cfg.Parallelism = DefaultParallelism
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

// Load reads and parses imgp.json, returning defaults if the file does not exist.
func Load() (*Config, error) {
	return LoadFrom(ConfigPath())
}

// Save writes the configuration to imgp.json.
func (c *Config) Save() error {
	if c.configPath == "" {
		c.configPath = ConfigPath()
	}
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

func osDefaultCacheDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "imgp", "cache")
	}
	return filepath.Join(os.TempDir(), "imgp-cache")
}

// EffectiveCacheDir returns the cache dir: config value, or OS default if empty.
func (c *Config) EffectiveCacheDir() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	return osDefaultCacheDir()
}
