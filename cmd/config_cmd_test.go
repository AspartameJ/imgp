package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
)

func TestSetConfigKey_MirrorMap(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil

	if err := setConfigKey(cfg, "mirror-map", "docker.io=m1|m2, quay.io=m3"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.MirrorMap == nil {
		t.Fatal("MirrorMap should not be nil")
	}
	if len(cfg.MirrorMap["docker.io"]) != 2 || cfg.MirrorMap["docker.io"][0] != "m1" || cfg.MirrorMap["docker.io"][1] != "m2" {
		t.Errorf("docker.io mirrors = %v, want [m1 m2]", cfg.MirrorMap["docker.io"])
	}
	if len(cfg.MirrorMap["quay.io"]) != 1 || cfg.MirrorMap["quay.io"][0] != "m3" {
		t.Errorf("quay.io mirrors = %v, want [m3]", cfg.MirrorMap["quay.io"])
	}
}

func TestSetConfigKey_MirrorMap_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "mirror-map", "badformat"); err == nil {
		t.Error("expected error for invalid mirror-map format")
	}
}

func TestSetConfigKey_Parallelism(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "parallelism", "8"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.Parallelism != 8 {
		t.Errorf("Parallelism = %d, want 8", cfg.Parallelism)
	}
}

func TestSetConfigKey_Parallelism_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "parallelism", "0"); err == nil {
		t.Error("expected error for parallelism = 0")
	}
	if err := setConfigKey(cfg, "parallelism", "abc"); err == nil {
		t.Error("expected error for non-numeric parallelism")
	}
}

func TestSetConfigKey_LayerTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "layer-timeout", "15"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.LayerTimeout != 15 {
		t.Errorf("LayerTimeout = %d, want 15", cfg.LayerTimeout)
	}
}

func TestSetConfigKey_Timeout(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "timeout", "60"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.Timeout != 60 {
		t.Errorf("Timeout = %d, want 60", cfg.Timeout)
	}
}

func TestSetConfigKey_Retry(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "retry", "5"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.Retry != 5 {
		t.Errorf("Retry = %d, want 5", cfg.Retry)
	}
}

func TestSetConfigKey_InsecureRegistries(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "insecure-registries", "reg1.io, reg2.io"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if len(cfg.InsecureRegistries) != 2 || cfg.InsecureRegistries[0] != "reg1.io" {
		t.Errorf("InsecureRegistries = %v, want [reg1.io reg2.io]", cfg.InsecureRegistries)
	}
}

func TestSetConfigKey_CacheDir(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "cache-dir", "/custom/cache"); err != nil {
		t.Fatalf("setConfigKey error = %v", err)
	}
	if cfg.CacheDir != "/custom/cache" {
		t.Errorf("CacheDir = %q, want /custom/cache", cfg.CacheDir)
	}
}

func TestSetConfigKey_Unknown(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "unknown-key", "value"); err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestConfigSetCmd_RunE(t *testing.T) {
	saveDir := cacheDir
	defer func() { cacheDir = saveDir }()
	cacheDir, _ = os.MkdirTemp("", "imgp-config-*")
	defer os.RemoveAll(cacheDir)

	cmd := &cobra.Command{}
	err := configSetCmd.RunE(cmd, []string{"parallelism", "8"})
	if err != nil {
		t.Fatalf("configSetCmd.RunE error = %v", err)
	}
}

func TestConfigSetCmd_RunE_InvalidKey(t *testing.T) {
	cmd := &cobra.Command{}
	err := configSetCmd.RunE(cmd, []string{"invalid-key", "value"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestSetConfigKey_LayerTimeout_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "layer-timeout", "abc"); err == nil {
		t.Error("expected error for non-numeric layer-timeout")
	}
}

func TestSetConfigKey_Timeout_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "timeout", "abc"); err == nil {
		t.Error("expected error for non-numeric timeout")
	}
}

func TestSetConfigKey_Retry_Invalid(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setConfigKey(cfg, "retry", "abc"); err == nil {
		t.Error("expected error for non-numeric retry")
	}
}

func TestConfigListCmd_RunE(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := &cobra.Command{}
	err := configListCmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("configListCmd.RunE error = %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	var readBuf bytes.Buffer
	readBuf.ReadFrom(r)

	output := readBuf.String()
	if !strings.Contains(output, "Mirror Map") {
		t.Errorf("config list should contain Mirror Map, got: %s", output)
	}
	if !strings.Contains(output, "Parallelism") {
		t.Errorf("config list should contain Parallelism, got: %s", output)
	}
}
