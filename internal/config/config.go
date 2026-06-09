package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type AuthConfig struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	PasswordEnv string `json:"password_env,omitempty"`
}

type Config struct {
	MirrorMap         map[string][]string `json:"mirror_map"`
	Auths             map[string]AuthConfig `json:"auths,omitempty"`
	InsecureRegistries []string           `json:"insecure_registries,omitempty"`
	Parallelism       int                 `json:"parallelism"`

	configPath string
}

func DefaultConfig() *Config {
	return &Config{
		MirrorMap: map[string][]string{
			"docker.io": {"docker.daocloud.io"},
			"quay.io":   {"quay.mirrors.daocloud.io"},
			"gcr.io":    {"gcr.mirrors.daocloud.io"},
		},
		Parallelism: 4,
	}
}

func ConfigPath() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "imgp.json")
	}
	// Fall back to current directory
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

	return cfg, nil
}

func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0644)
}

func (c *Config) ExeDir() string {
	return filepath.Dir(c.configPath)
}
