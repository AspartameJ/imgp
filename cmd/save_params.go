package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/util"
)

type saveParams struct {
	parallelism    int
	layerTimeout   int
	overallTimeout int
	retry          int
	targetPlatform string
	password       string
	outputPath     string
	gzip           bool
	quiet          bool
	noCache        bool
}

func cmdCacheDir(cfg *config.Config) string {
	if cacheDir != "" {
		return cacheDir
	}
	return cfg.EffectiveCacheDir()
}

func resolvePlatform(input string) (string, error) {
	if input == "" {
		return "linux/amd64", nil
	}
	parts := strings.Split(input, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", fmt.Errorf("invalid platform format %q, expected os/arch or os/arch/variant (e.g. linux/amd64, linux/arm64/v8)", input)
	}
	for _, x := range parts {
		if x == "" {
			return "", fmt.Errorf("invalid platform format %q: empty segment", input)
		}
	}
	if !util.IsValidArch(parts[1]) {
		if s := util.ArchSuggestion(parts[1]); s != "" {
			return "", fmt.Errorf("unknown architecture %q in platform %q, did you mean %q?", parts[1], input, s)
		}
		return "", fmt.Errorf("unknown architecture %q in platform %q, valid values: 386, amd64, arm, arm64, loong64, mips, mips64, mips64le, mipsle, ppc64, ppc64le, riscv64, s390x, wasm", parts[1], input)
	}
	return input, nil
}

func resolveOutputPath(image, targetPlatform string) string {
	if output != "" {
		return filepath.Clean(output)
	}
	baseName := strings.ReplaceAll(strings.ReplaceAll(image, "/", "_"), ":", "_")
	plat := strings.ReplaceAll(targetPlatform, "/", "-")
	ext := ".tar"
	if gzip {
		ext = ".tar.gz"
	}
	return fmt.Sprintf("%s_%s%s", baseName, plat, ext)
}

func resolvePassword() string {
	if password != "" {
		return password
	}
	if passwordEnv != "" {
		return os.Getenv(passwordEnv)
	}
	return ""
}

func resolveSaveParams(cmd *cobra.Command, cfg *config.Config, image string) (saveParams, error) {
	var p saveParams

	p.parallelism = cfg.Parallelism
	if parallelism > 0 {
		p.parallelism = parallelism
	}
	if p.parallelism < 1 {
		p.parallelism = config.DefaultParallelism
	}

	if cmd.Flags().Changed("layer-timeout") {
		p.layerTimeout = layerTimeoutMin
	} else if cfg.LayerTimeout != nil {
		p.layerTimeout = *cfg.LayerTimeout
	} else {
		p.layerTimeout = 30
	}

	p.overallTimeout = timeoutMin
	if !cmd.Flags().Changed("timeout") && cfg.Timeout != nil {
		p.overallTimeout = *cfg.Timeout
	}

	p.retry = cfg.Retry
	if cmd.Flags().Changed("retry") {
		p.retry = retryCount
	}
	if p.retry > 30 {
		p.retry = 30
	}

	targetPlatform, err := resolvePlatform(platform)
	if err != nil {
		return p, err
	}
	p.targetPlatform = targetPlatform
	p.password = resolvePassword()
	p.outputPath = resolveOutputPath(image, p.targetPlatform)
	p.gzip = gzip
	p.quiet = quiet
	p.noCache = noCache
	return p, nil
}
