package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
)

var (
	platform    string
	output      string
	username    string
	password    string
	passwordEnv string
	insecure    bool
	parallelism int
	quiet       bool
	noCache     bool
	cacheDir    string
	timeoutMin  int
	layerTimeoutMin int
	retryCount  int
)

var rootCmd = &cobra.Command{
	Use:     "imgp",
	Short:   "Cross-platform Docker image pull and save tool",
	Version: "1.3.0",
}

var saveCmd = &cobra.Command{
	Use:   "save [image]",
	Short: "Pull a Docker image and save it as a tar archive",
	Long: `Pull a Docker image from a registry (with mirror acceleration support)
and save it as a Docker-compatible tar archive.

Supports multi-architecture images, parallel downloads, and resume.

Examples:
  imgp save hello-world:latest -o hello-world.tar
  imgp save hello-world:latest --platform linux/arm64 -o hello-world-arm64.tar
  imgp save myuser/myapp:latest -o myapp.tar --username myuser --password-env`,
	Args: cobra.ExactArgs(1),
	RunE: runSave,
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage layer cache",
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		cd := config.CacheDir()
		var totalSize int64
		var fileCount int
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				fi, err := e.Info()
				if err == nil && !e.IsDir() {
					totalSize += fi.Size()
					fileCount++
				}
			}
		}
		fmt.Printf("Cache directory: %s\n", cd)
		fmt.Printf("Cached layers:   %d\n", fileCount)
		fmt.Printf("Total size:      %s\n", formatBytes(totalSize))
		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached layers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cd := config.CacheDir()
		var removed int
		var freed int64
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				fi, err := e.Info()
				if err == nil && !e.IsDir() {
					freed += fi.Size()
					removed++
				}
				os.RemoveAll(filepath.Join(cd, e.Name()))
			}
		}
		fmt.Printf("Cleared %d cached layers (%s)\n", removed, formatBytes(freed))
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage imgp configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in imgp.json.

Supported keys:
  mirror-map          Comma-separated registry=mirror pairs (e.g., docker.io=mirror1|mirror2,quay.io=mirror)
  insecure-registries Comma-separated registry hostnames
  parallelism         Number of parallel downloads (default: 4)
  layer-timeout       Per-layer download timeout in minutes (default: 30)
  timeout             Overall operation timeout in minutes (default: 0 = no limit)
  retry               Number of retries on network errors (default: 2)`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		key, value := args[0], args[1]
		switch key {
		case "mirror-map":
			for _, pair := range strings.Split(value, ",") {
				pair = strings.TrimSpace(pair)
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid mirror-map format, expected: registry1=mirror1,... or registry1=mirror1|mirror2")
				}
				reg := strings.TrimSpace(parts[0])
				mirrors := strings.Split(strings.TrimSpace(parts[1]), "|")
				for i := range mirrors {
					mirrors[i] = strings.TrimSpace(mirrors[i])
				}
				cfg.MirrorMap[reg] = mirrors
			}
		case "insecure-registries":
			cfg.InsecureRegistries = strings.Split(value, ",")
		case "parallelism":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("parallelism must be a positive integer")
			}
			cfg.Parallelism = n
		case "layer-timeout":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("layer-timeout must be a positive integer")
			}
			cfg.LayerTimeout = n
		case "timeout":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 0 {
				return fmt.Errorf("timeout must be 0 or a positive integer")
			}
			cfg.Timeout = n
		case "retry":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 0 {
				return fmt.Errorf("retry must be 0 or a positive integer")
			}
			cfg.Retry = n
		default:
			return fmt.Errorf("unknown key: %s (supported: mirror-map, insecure-registries, parallelism, layer-timeout, timeout, retry)", key)
		}
		return cfg.Save()
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		data := fmt.Sprintf("Mirror Map: %v\n", cfg.MirrorMap)
		data += fmt.Sprintf("Insecure Registries: %v\n", cfg.InsecureRegistries)
		data += fmt.Sprintf("Parallelism: %d\n", cfg.Parallelism)
		data += fmt.Sprintf("Layer Timeout: %d min\n", cfg.LayerTimeout)
		data += fmt.Sprintf("Timeout: %d min\n", cfg.Timeout)
		data += fmt.Sprintf("Retry: %d\n", cfg.Retry)
		_, err = fmt.Print(data)
		return err
	},
}

func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(guiCmd)
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheInfoCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)

	saveCmd.Flags().StringVarP(&platform, "platform", "p", "",
		"Target platform (e.g., linux/amd64, linux/arm64, windows/amd64)")
	saveCmd.Flags().StringVarP(&output, "output", "o", "", "Output tar file path")
	saveCmd.Flags().StringVar(&username, "username", "", "Registry username")
	saveCmd.Flags().StringVar(&password, "password", "", "Registry password (use --password-env for security)")
	saveCmd.Flags().StringVar(&passwordEnv, "password-env", "IMG_REGISTRY_PASSWORD",
		"Environment variable name for registry password")
	saveCmd.Flags().BoolVar(&insecure, "insecure", false,
		"Allow insecure registry connections (skip TLS verify)")
	saveCmd.Flags().IntVarP(&parallelism, "parallel", "P", 0,
		"Number of parallel layer downloads (default: from config, or 4)")
	saveCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet mode, less output")
	saveCmd.Flags().BoolVar(&noCache, "no-cache", false, "Ignore cached layers, force re-download")
	saveCmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (OS-specific default: %LOCALAPPDATA%/imgp/cache, ~/.cache/imgp, ~/Library/Caches/imgp)")
	saveCmd.Flags().IntVar(&timeoutMin, "timeout", 0, "Overall timeout in minutes (0 = no limit)")
	saveCmd.Flags().IntVar(&layerTimeoutMin, "layer-timeout", 30, "Per-layer download timeout in minutes")
	saveCmd.Flags().IntVar(&retryCount, "retry", 2, "Number of retries on network errors (0 = no retry)")
}

func cmdCacheDir() string {
	if cacheDir != "" {
		return cacheDir
	}
	return config.CacheDir()
}

func runSave(cmd *cobra.Command, args []string) error {
	image := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	par := cfg.Parallelism
	if parallelism > 0 {
		par = parallelism
	}

	lt := cfg.LayerTimeout
	if lt == 0 {
		lt = 30
	}
	if layerTimeoutMin > 0 {
		lt = layerTimeoutMin
	}

	to := timeoutMin
	if to == 0 && cfg.Timeout > 0 {
		to = cfg.Timeout
	}

	rt := cfg.Retry
	if retryCount > 0 {
		rt = retryCount
	} else if retryCount == 0 && cfg.Retry == 0 {
		rt = 2
	}

	targetPlatform := platform
	if targetPlatform == "" {
		targetPlatform = "linux/amd64"
	} else {
		parts := strings.Split(targetPlatform, "/")
		if len(parts) < 2 || len(parts) > 3 {
			return fmt.Errorf("invalid platform format %q, expected os/arch or os/arch/variant (e.g. linux/amd64, linux/arm64/v8)", targetPlatform)
		}
	}

	// Resolve password
	pass := password
	if pass == "" && passwordEnv != "" {
		pass = os.Getenv(passwordEnv)
	}

	// Default output name
	outPath := output
	if outPath == "" {
		name := strings.ReplaceAll(strings.ReplaceAll(image, "/", "_"), ":", "_")
		plat := strings.ReplaceAll(targetPlatform, "/", "-")
		outPath = fmt.Sprintf("%s_%s.tar", name, plat)
	}

	// Create cache directory
	cd := cmdCacheDir()
	if err := os.MkdirAll(cd, 0755); err != nil {
		return fmt.Errorf("create cache: %w", err)
	}

	// Create registry client
	client := registry.NewClient(cfg).WithAuth(username, pass).WithInsecure(insecure)

	// Phase 1: Pull
	if !quiet {
		fmt.Printf("Pulling %s (%s)\n", image, targetPlatform)
	}

	img, ref, err := client.FetchImage(cmd.Context(), image, targetPlatform)
	if err != nil {
		return fmt.Errorf("fetch image: %w", err)
	}

	if !quiet {
		fmt.Println("Image manifest fetched, downloading layers...")
	}

	layerFetcher := client.NewLayerFetcher(ref)

	imgLayers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("get layers: %w", err)
	}

	tasks := make([]puller.LayerTask, len(imgLayers))
	for i, l := range imgLayers {
		digest, err := l.Digest()
		if err != nil {
			return fmt.Errorf("get layer %d digest: %w", i, err)
		}
		size, err := l.Size()
		if err != nil {
			return fmt.Errorf("get layer %d size: %w", i, err)
		}
		dHex := digest.Hex
		tasks[i] = puller.LayerTask{
			Index:     i,
			DigestHex: dHex,
			Size:      size,
			OpenLayer: func(ctx context.Context) (io.ReadCloser, error) {
				return layerFetcher(ctx, dHex)
			},
		}
	}

	pl := puller.NewPuller(cd).WithNoCache(noCache).WithLayerTimeout(time.Duration(lt)*time.Minute).WithRetry(rt)

	ctx := cmd.Context()
	if to > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(to)*time.Minute)
		defer cancel()
	}

	eventCh, err := pl.Pull(ctx, tasks, par)
	if err != nil {
		return fmt.Errorf("start pull: %w", err)
	}

	progress := newProgressDisplay(quiet)
	pullDone := progress.startPull(eventCh, tasks)
	<-pullDone
	if progress.hasError {
		for _, ls := range progress.layers {
			if ls.status == "error" && ls.errMsg != "" {
				return fmt.Errorf("layer %s download failed: %s", shorten(ls.digest, 12), ls.errMsg)
			}
		}
		return fmt.Errorf("layer download failed")
	}

	// Phase 2: Export
	if !quiet {
		fmt.Printf("\nExporting to %s\n", outPath)
	}

	cachePathFn := func(digest string) string {
		return filepath.Join(cd, digest+".gz")
	}

	if !quiet {
		fmt.Printf("\r  exporting: 0%%")
	}
	err = saver.Export(ref, img, outPath, cachePathFn,
		func(completed, total int64) {
			if quiet {
				return
			}
			percent := float64(0)
			if total > 0 {
				percent = float64(completed) / float64(total) * 100
			}
			fmt.Printf("\r  exporting: %.0f%% | %s / %s",
				percent, formatBytes(completed), formatBytes(total))
		},
	)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if !quiet {
		fmt.Println()
	}

	if !quiet {
		fmt.Printf("\nDone: %s (%s) saved to %s\n", image, targetPlatform, outPath)
	} else {
		fmt.Println(outPath)
	}

	return nil
}

type layerState struct {
	index   int
	digest  string
	total   int64
	current int64
	status  string
	errMsg  string
}

type progressDisplay struct {
	quiet    bool
	useANSI bool
	mu       sync.Mutex
	layers   []layerState
	total    int64
	hasError bool
}

func isTerminal() bool {
	fi, _ := os.Stdout.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) != 0
}

func newProgressDisplay(quiet bool) *progressDisplay {
	return &progressDisplay{quiet: quiet, useANSI: isTerminal()}
}

func (p *progressDisplay) startPull(eventCh <-chan puller.PullEvent, tasks []puller.LayerTask) <-chan struct{} {
	quit := make(chan struct{})

	if p.quiet {
		go func() {
			defer close(quit)
			for range eventCh {
			}
		}()
		return quit
	}

	defer close(quit)

	p.layers = make([]layerState, len(tasks))
	totalLayers := len(tasks)

	p.total = 0
	for i, t := range tasks {
		p.layers[i] = layerState{index: t.Index, status: "pending"}
		p.total += t.Size
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for evt := range eventCh {
			p.mu.Lock()
			if evt.Index < len(p.layers) {
				p.layers[evt.Index].digest = evt.Digest
				p.layers[evt.Index].total = evt.Total
				if evt.Err != nil {
					p.hasError = true
					p.layers[evt.Index].status = "error"
					p.layers[evt.Index].errMsg = evt.Err.Error()
				} else {
					p.layers[evt.Index].current = evt.Bytes
					p.layers[evt.Index].status = evt.Status
				}
			}
			p.mu.Unlock()
		}
	}()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	prevLayers := 0
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			allDone := true
			var currentBytes int64
			doneLayers := 0
			for _, ls := range p.layers {
				switch ls.status {
				case "done", "cached":
					doneLayers++
					currentBytes += ls.total
				case "downloading":
					allDone = false
					currentBytes += ls.current
				default:
					allDone = false
				}
			}

			percent := float64(0)
			if p.total > 0 {
				percent = float64(currentBytes) / float64(p.total) * 100
			}

			if p.useANSI && prevLayers > 0 {
				fmt.Printf("\033[%dA", prevLayers)
			}

			if p.useANSI {
				fmt.Printf("\033[2K\r  layers: [%d/%d] %.1f%% | %s / %s\n",
					doneLayers, totalLayers, percent,
					formatBytes(currentBytes), formatBytes(p.total))
			} else {
				fmt.Printf("\r  layers: [%d/%d] %.1f%% | %s / %s",
					doneLayers, totalLayers, percent,
					formatBytes(currentBytes), formatBytes(p.total))
			}

			if p.useANSI {
				prevLayers = 1 + totalLayers
				for _, ls := range p.layers {
					bar := renderBar(ls.current, ls.total, 30)
					switch ls.status {
					case "cached":
						fmt.Printf("\033[2K\r    ✓ %s %s (cached)\n", shorten(ls.digest, 12), bar)
					case "done":
						fmt.Printf("\033[2K\r    ✓ %s %s\n", shorten(ls.digest, 12), bar)
					case "downloading":
						fmt.Printf("\033[2K\r    ◌ %s %s %s/%s\n",
							shorten(ls.digest, 12), bar,
							formatBytes(ls.current), formatBytes(ls.total))
				case "error":
					msg := "download failed"
					if ls.errMsg != "" {
						msg = ls.errMsg
					}
					fmt.Printf("\033[2K\r    ✗ %s %s\n", shorten(ls.digest, 12), msg)
					default:
						fmt.Printf("\033[2K\r    · %s waiting...\n", shorten(ls.digest, 12))
					}
				}
			}
			p.mu.Unlock()

			if allDone {
				return quit
			}

		case <-readerDone:
			return quit
		}
	}
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	switch exp {
	case 0:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(div))
	case 1:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(div))
	case 2:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(div))
	default:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(div))
	}
}

func renderBar(current, total int64, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := int(float64(current) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	percent := float64(0)
	if total > 0 {
		percent = float64(current) / float64(total) * 100
	}
	return fmt.Sprintf("%s %.0f%%", bar, percent)
}
