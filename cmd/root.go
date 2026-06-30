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

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
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
	Version: "2.0.0",
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

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage layer cache",
	Long:  "View and manage the layer cache to avoid re-downloading layers on repeat pulls.",
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache usage",
	Long:  "Display the cache directory path, number of cached layers, and total disk usage.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cd := cmdCacheDir()
		var totalSize int64
		var fileCount int
		entries, err := os.ReadDir(cd)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Cache directory: %s (not yet created)\n", cd)
				fmt.Printf("Cached layers:   0\n")
				fmt.Printf("Total size:      0 B\n")
				return nil
			}
			return fmt.Errorf("read cache directory: %w", err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".gz") {
				continue
			}
			if e.IsDir() {
				continue
			}
			fi, err := e.Info()
			if err == nil {
				totalSize += fi.Size()
				fileCount++
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
	Long:  "Remove all cached layer files to free up disk space.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cd := cmdCacheDir()
		var removed int
		var freed int64
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".gz") {
					continue
				}
				if e.IsDir() {
					continue
				}
				if err := os.Remove(filepath.Join(cd, e.Name())); err != nil {
					return fmt.Errorf("clear cache: %w", err)
				}
				fi, err := e.Info()
				if err == nil {
					freed += fi.Size()
					removed++
				}
			}
		}
		fmt.Printf("Cleared %d cached layers (%s)\n", removed, formatBytes(freed))
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage imgp configuration",
	Long:  "View and modify imgp configuration stored in imgp.json next to the binary.",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in imgp.json.

Supported keys:
  mirror-map          Comma-separated registry=mirror pairs (e.g., docker.io=mirror1|mirror2,quay.io=mirror)
  insecure-registries Comma-separated registry hostnames
  parallelism         Number of parallel downloads (default: 4)
  layer-timeout       Per-layer download timeout in minutes (default: 30, 0 = no limit)
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
			var list []string
			for _, s := range strings.Split(value, ",") {
				if s = strings.TrimSpace(s); s != "" {
					list = append(list, s)
				}
			}
			cfg.InsecureRegistries = list
		case "parallelism":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 1 {
				return fmt.Errorf("parallelism must be a positive integer")
			}
			cfg.Parallelism = n
		case "layer-timeout":
			n := 0
			if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n < 0 {
				return fmt.Errorf("layer-timeout must be 0 or a positive integer")
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
	Long:  "Display all configuration values: mirror map, parallelism, timeouts, retry, and insecure registries.",
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
		cancel()
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheInfoCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)

	saveCmd.Flags().StringVarP(&platform, "platform", "p", "",
		"Target platform (default: linux/amd64; e.g., linux/arm64, windows/amd64)")
	saveCmd.Flags().StringVarP(&output, "output", "o", "", "Output tar file path")
	saveCmd.Flags().StringVar(&username, "username", "", "Registry username")
	saveCmd.Flags().StringVar(&password, "password", "", "Registry password (use --password-env for security)")
	saveCmd.Flags().StringVar(&passwordEnv, "password-env", "IMG_REGISTRY_PASSWORD",
		"Environment variable name for registry password (--password takes priority if set)")
	saveCmd.Flags().BoolVar(&insecure, "insecure", false,
		"Allow insecure registry connections (skip TLS verify)")
	saveCmd.Flags().IntVarP(&parallelism, "parallel", "P", 0,
		"Number of parallel layer downloads (default: from config, or 4)")
	saveCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Quiet mode, less output")
	saveCmd.Flags().BoolVar(&noCache, "no-cache", false, "Ignore cached layers, force re-download")
	saveCmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (default: OS-specific: %LOCALAPPDATA%/imgp/cache on Windows, $XDG_CACHE_HOME/imgp or ~/.cache/imgp on Linux, ~/Library/Caches/imgp on macOS)")
	saveCmd.Flags().IntVar(&timeoutMin, "timeout", 0, "Overall timeout in minutes (0 = no limit)")
	saveCmd.Flags().IntVar(&layerTimeoutMin, "layer-timeout", 30, "Per-layer download timeout in minutes (0 = no limit)")
	saveCmd.Flags().IntVar(&retryCount, "retry", 2, "Number of retries on network errors (0 = no retry)")

	cacheCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (default: OS-specific path)")
}

func cmdCacheDir() string {
	if cacheDir != "" {
		return cacheDir
	}
	return config.CacheDir()
}

func runSave(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if len(args) == 1 {
		return runSaveOne(cmd, cfg, args[0], "")
	}
	if output != "" {
		return fmt.Errorf("cannot use -o/--output with multiple images; each image gets an auto-named tar")
	}
	total := len(args)
	var errs []string
	for i, image := range args {
		if cmd.Context().Err() != nil {
			errs = append(errs, fmt.Sprintf("%s: canceled by user", image))
			break
		}
		if i > 0 {
			if isTerminal() {
				fmt.Print("\033[J")
			}
			fmt.Println()
		}
		batchInfo := fmt.Sprintf("[%d/%d] ", i+1, total)
		if err := runSaveOne(cmd, cfg, image, batchInfo); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", image, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("batch download completed with errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func runSaveOne(cmd *cobra.Command, cfg *config.Config, image string, batchInfo string) error {

	par := cfg.Parallelism
	if parallelism > 0 {
		par = parallelism
	}
	if par < 1 {
		par = 4
	}

	lt := cfg.LayerTimeout
	if cmd.Flags().Changed("layer-timeout") {
		lt = layerTimeoutMin
	} else if lt == 0 {
		lt = 30
	}

	to := timeoutMin
	if !cmd.Flags().Changed("timeout") && to == 0 && cfg.Timeout > 0 {
		to = cfg.Timeout
	}

	rt := cfg.Retry
	if cmd.Flags().Changed("retry") {
		rt = retryCount
	}
	if rt > 30 {
		rt = 30
	}

	targetPlatform := platform
	if targetPlatform == "" {
		targetPlatform = "linux/amd64"
	} else {
		parts := strings.Split(targetPlatform, "/")
		if len(parts) < 2 || len(parts) > 3 {
			return fmt.Errorf("invalid platform format %q, expected os/arch or os/arch/variant (e.g. linux/amd64, linux/arm64/v8)", targetPlatform)
		}
		for _, p := range parts {
			if p == "" {
				return fmt.Errorf("invalid platform format %q: empty segment", targetPlatform)
			}
		}
		if !isValidArch(parts[1]) {
			if s := archSuggestion(parts[1]); s != "" {
				return fmt.Errorf("unknown architecture %q in platform %q, did you mean %q?", parts[1], targetPlatform, s)
			}
			return fmt.Errorf("unknown architecture %q in platform %q, valid values: 386, amd64, arm, arm64, loong64, mips, mips64, mips64le, mipsle, ppc64, ppc64le, riscv64, s390x, wasm", parts[1], targetPlatform)
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
	} else {
		outPath = filepath.Clean(outPath)
	}

	// Create cache directory
	cd := cmdCacheDir()
	if err := os.MkdirAll(cd, 0755); err != nil {
		return fmt.Errorf("create cache: %w", err)
	}

	// Create registry client
	client := registry.NewClient(cfg).WithAuth(username, pass).WithInsecure(insecure).WithRetry(rt)

	// Apply overall timeout to entire operation (fetch + pull)
	ctx := cmd.Context()
	if to > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(to)*time.Minute)
		defer cancel()
	}

	// Phase 1: Pull
	if !quiet {
		if isTerminal() {
			fmt.Printf("%s%s %s (%s)\n", batchInfo, colorCyan("⟳ Pulling"), image, targetPlatform)
		} else {
			fmt.Printf("%sPulling %s (%s)\n", batchInfo, image, targetPlatform)
		}
	}

	img, ref, err := client.FetchImage(ctx, image, targetPlatform)
	if err != nil {
		if strings.Contains(err.Error(), "dial tcp") || strings.Contains(err.Error(), "i/o timeout") {
			return fmt.Errorf("fetch image: %w\n  		tip: check network/proxy or try with a mirror (e.g. `imgp config set mirror-map registry.k8s.io=m.daocloud.io/registry.k8s.io`)", err)
		}
		return fmt.Errorf("fetch image: %w", err)
	}

	origRef, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("parse image reference: %w", err)
	}

	if !quiet {
		if isTerminal() {
			fmt.Printf("%s\n", colorGreen("✓ Image manifest fetched, downloading layers..."))
		} else {
			fmt.Println("Image manifest fetched, downloading layers...")
		}
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

	pl := puller.NewPuller(cd).WithNoCache(noCache).WithLayerTimeout(time.Duration(lt) * time.Minute).WithRetry(rt)

	eventCh, err := pl.Pull(ctx, tasks, par)
	if err != nil {
		return fmt.Errorf("start pull: %w", err)
	}

	progress := newProgressDisplay(quiet)
	pullDone := progress.startPull(eventCh, tasks)
	<-pullDone
	progress.mu.Lock()
	hasError := progress.hasError
	layers := make([]layerState, len(progress.layers))
	copy(layers, progress.layers)
	progress.mu.Unlock()
	if hasError {
		var errs []string
		for _, ls := range layers {
			if ls.status == "error" {
				if ls.errMsg != "" {
					errs = append(errs, fmt.Sprintf("layer %s: %s", shorten(ls.digest, 12), ls.errMsg))
				} else {
					errs = append(errs, fmt.Sprintf("layer %s: unknown error", shorten(ls.digest, 12)))
				}
			}
		}
		return fmt.Errorf("layer download failed:\n  %s", strings.Join(errs, "\n  "))
	}

	// Phase 2: Export
	if !quiet {
		if isTerminal() {
			fmt.Printf("\n%s\n", colorCyan("⟳ Exporting to ")+outPath)
		} else {
			fmt.Printf("\nExporting to %s\n", outPath)
		}
	}

	cachePathFn := func(digest string) string {
		return filepath.Join(cd, digest+".gz")
	}

	if !quiet {
		fmt.Printf("\r  exporting: 0%%")
	}
	err = saver.Export(ctx, origRef, img, outPath, cachePathFn,
		func(completed, total int64) {
			if quiet {
				return
			}
			percent := float64(completed) / float64(total) * 100
			fmt.Printf("\r  exporting: %.0f%% | %s / %s",
				percent, formatBytes(completed), formatBytes(total))
		},
	)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if !quiet {
		if isTerminal() {
			fmt.Printf("\n%s %s (%s) saved to %s\n", colorGreen("✓ Done:"), image, targetPlatform, outPath)
		} else {
			fmt.Printf("\nDone: %s (%s) saved to %s\n", image, targetPlatform, outPath)
		}
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
	useANSI  bool
	mu       sync.Mutex
	layers   []layerState
	total    int64
	hasError bool
}

func colorGreen(s string) string  { return "\033[32m" + s + "\033[0m" }
func colorRed(s string) string    { return "\033[31m" + s + "\033[0m" }
func colorCyan(s string) string   { return "\033[36m" + s + "\033[0m" }
func colorYellow(s string) string { return "\033[33m" + s + "\033[0m" }

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
		p.layers = make([]layerState, len(tasks))
		go func() {
			defer func() { recover(); close(quit) }()
			for evt := range eventCh {
				p.mu.Lock()
				if evt.Err != nil {
					p.hasError = true
				}
				if evt.Index < len(p.layers) {
					p.layers[evt.Index].digest = evt.Digest
					p.layers[evt.Index].status = evt.Status
					if evt.Err != nil {
						p.layers[evt.Index].status = "error"
						p.layers[evt.Index].errMsg = evt.Err.Error()
					}
				}
				p.mu.Unlock()
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
		defer func() { recover(); close(readerDone) }()
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

			var buf strings.Builder
			if p.useANSI && prevLayers > 0 {
				fmt.Fprintf(&buf, "\033[%dA", prevLayers)
			}

			if p.useANSI {
				fmt.Fprintf(&buf, "\033[2K\r  layers: [%d/%d] %.1f%% | %s / %s\n",
					doneLayers, totalLayers, percent,
					formatBytes(currentBytes), formatBytes(p.total))
			} else {
				buf.WriteString(fmt.Sprintf("\r  layers: [%d/%d] %.1f%% | %s / %s",
					doneLayers, totalLayers, percent,
					formatBytes(currentBytes), formatBytes(p.total)))
			}

			if p.useANSI {
				prevLayers = 1 + totalLayers
				for _, ls := range p.layers {
					bar := renderBar(ls.current, ls.total, 30)
					switch ls.status {
					case "cached":
						fmt.Fprintf(&buf, "\033[2K\r    %s %s %s\n", colorGreen("✓"), shorten(ls.digest, 12), colorGreen("(cached)"))
					case "done":
						fmt.Fprintf(&buf, "\033[2K\r    %s %s %s\n", colorGreen("✓"), shorten(ls.digest, 12), bar)
					case "downloading":
						fmt.Fprintf(&buf, "\033[2K\r    %s %s %s %s/%s\n",
							colorCyan("◌"), shorten(ls.digest, 12), bar,
							formatBytes(ls.current), formatBytes(ls.total))
					case "error":
						msg := "download failed"
						if ls.errMsg != "" {
							msg = ls.errMsg
						}
						fmt.Fprintf(&buf, "\033[2K\r    %s %s %s\n", colorRed("✗"), shorten(ls.digest, 12), colorRed(msg))
					default:
						fmt.Fprintf(&buf, "\033[2K\r    %s %s waiting...\n", colorYellow("·"), shorten(ls.digest, 12))
					}
				}
			}
			p.mu.Unlock()
			fmt.Print(buf.String())

			if allDone {
				<-readerDone
				return quit
			}

		case <-readerDone:
			return quit
		}
	}
}

func shorten(s string, n int) string {
	if n <= 0 {
		return ""
	}
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

func isValidArch(arch string) bool {
	switch arch {
	case "386", "amd64", "arm", "arm64", "loong64", "mips",
		"mips64", "mips64le", "mipsle", "ppc64", "ppc64le",
		"riscv64", "s390x", "wasm":
		return true
	}
	return false
}

func archSuggestion(arch string) string {
	switch arch {
	case "aarch64":
		return "arm64"
	case "x86_64":
		return "amd64"
	case "amd":
		return "amd64"
	case "i386", "i686":
		return "386"
	case "armv7", "armv7l":
		return "arm"
	case "armv8", "arm64v8":
		return "arm64"
	}
	return ""
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
	percent := float64(current) / float64(total) * 100
	return fmt.Sprintf("%s %.0f%%", bar, percent)
}
