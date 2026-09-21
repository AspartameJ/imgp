package cmd

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/version"
)

var (
	platform        string
	output          string
	username        string
	password        string
	passwordEnv     string
	insecure        bool
	parallelism     int
	quiet           bool
	noCache         bool
	resume          bool
	gzip            bool
	cacheDir        string
	timeoutMin      int
	layerTimeoutMin int
	retryCount      int
)

var rootCmd = &cobra.Command{
	Use:   "imgp",
	Short: "Cross-platform Docker image pull and save tool",
	Long: `A fast, cross-platform tool for pulling Docker images and saving them as tar archives.

Supports multiple architectures (default: linux/amd64), parallel downloads,
built-in mirror acceleration, and layer caching.`,
}

var saveCmd = &cobra.Command{
	Use:   "save [image...]",
	Short: "Pull Docker images and save them as tar archives",
	Long: `Pull Docker images from a registry (with mirror acceleration support)
and save them as Docker-compatible tar archives.

Supports multi-architecture images, parallel downloads, and layer caching.
Multiple images can be specified for batch download.

Examples:
  imgp save hello-world:latest -o hello-world.tar
  imgp save hello-world:latest --platform linux/arm64 -o hello-world-arm64.tar
  imgp save myuser/myapp:latest -o myapp.tar --username myuser --password-env
  imgp save registry.k8s.io/kube-apiserver:v1.34.9 registry.k8s.io/kube-scheduler:v1.34.9`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSave,
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = version.Version
	rootCmd.AddCommand(saveCmd)

	saveCmd.Flags().StringVarP(&platform, "platform", "p", "",
		"Target platform (default: linux/amd64; e.g., linux/arm64, windows/amd64)")
	saveCmd.Flags().StringVarP(&output, "output", "o", "", "Output tar file path")
	saveCmd.Flags().StringVar(&username, "username", "", "Registry username")
	saveCmd.Flags().StringVar(&password, "password", "", "Registry password (use --password-env for security)")
	saveCmd.Flags().StringVar(&passwordEnv, "password-env", "",
		"Environment variable name for registry password (--password takes priority if set)")
	saveCmd.Flags().BoolVar(&insecure, "insecure", false,
		"Allow insecure registry connections (skip TLS verify)")
	saveCmd.Flags().IntVarP(&parallelism, "parallel", "P", 0,
		"Number of parallel layer downloads (default: from config, or 4)")
	saveCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet mode, less output")
	saveCmd.Flags().BoolVar(&noCache, "no-cache", false, "Ignore cached layers, force re-download")
	saveCmd.Flags().BoolVar(&resume, "resume", false, "Resume interrupted layer downloads via HTTP Range requests")
	saveCmd.Flags().BoolVarP(&gzip, "gzip", "z", false, "Gzip-compress the output tar file")
	saveCmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (default: OS-specific: %LOCALAPPDATA%/imgp/cache on Windows, $XDG_CACHE_HOME/imgp or ~/.cache/imgp on Linux, ~/Library/Caches/imgp on macOS)")
	saveCmd.Flags().IntVar(&timeoutMin, "timeout", 0, "Overall timeout in minutes (0 = no limit)")
	saveCmd.Flags().IntVar(&layerTimeoutMin, "layer-timeout", 30, "Per-layer download timeout in minutes (0 = no limit)")
	saveCmd.Flags().IntVar(&retryCount, "retry", 2, "Number of retries on network errors (0 = no retry)")
}
