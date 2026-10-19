package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitcode.com/DonaldTom/imgp/internal/util"
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
	LayerTimeout       *int                  `json:"layer_timeout,omitempty"`
	Timeout            *int                  `json:"timeout,omitempty"`
	Retry              int                   `json:"retry"`
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
	def := DefaultConfig()
	// MirrorMap is left nil so an absent key is distinguishable from an
	// explicitly empty map; the scalars are pre-seeded so that "retry": 0
	// (a valid value) is told apart from a missing field.
	cfg := &Config{configPath: path, Parallelism: def.Parallelism, Retry: def.Retry}

	data, err := os.ReadFile(path) //nolint:gosec // path is the resolved imgp.json location
	if err != nil {
		if os.IsNotExist(err) {
			def.configPath = path
			return def, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	switch {
	case cfg.MirrorMap == nil:
		cfg.MirrorMap = def.MirrorMap
	case len(cfg.MirrorMap) == 0:
		// Explicitly empty map: all mirrors disabled by the user.
	default:
		// Built-in mirrors stay available; config entries (normalized so that
		// aliases like index.docker.io land on the same key) take precedence.
		merged := make(map[string][]string, len(def.MirrorMap)+len(cfg.MirrorMap))
		for k, v := range def.MirrorMap {
			merged[k] = v
		}
		for k, v := range normalizeKeys(cfg.MirrorMap) {
			merged[k] = v
		}
		cfg.MirrorMap = merged
	}
	if cfg.Auths != nil {
		cfg.Auths = normalizeKeys(cfg.Auths)
	}
	if cfg.Parallelism < 1 {
		cfg.Parallelism = def.Parallelism
	}
	if cfg.LayerTimeout != nil && *cfg.LayerTimeout < 0 {
		z := 0
		cfg.LayerTimeout = &z
	}
	if cfg.Timeout != nil && *cfg.Timeout < 0 {
		z := 0
		cfg.Timeout = &z
	}
	if cfg.Retry < 0 {
		cfg.Retry = 0
	}

	return cfg, nil
}

// normalizeKeys canonicalises registry keys ("*" is preserved) so that aliases
// such as index.docker.io or host:443 match the lookup side.
func normalizeKeys[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		if k == "*" {
			out[k] = v
			continue
		}
		out[util.NormalizeRegistry(k)] = v
	}
	return out
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
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "imgp")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "imgp")
	}
	if d, err := os.UserHomeDir(); err == nil {
		return filepath.Join(d, ".cache", "imgp")
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
