package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
)

//go:embed web
var webFS embed.FS

var guiPort = "19191"

type layerStatus struct {
	Index  int    `json:"index"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
	Total  int64  `json:"total"`
	Status string `json:"status"`
}

type progressData struct {
	Layers      []layerStatus `json:"layers"`
	TotalBytes  int64         `json:"totalBytes"`
	DoneBytes   int64         `json:"doneBytes"`
	TotalLayers int           `json:"totalLayers"`
	DoneLayers  int           `json:"doneLayers"`
	Phase       string        `json:"phase"`
	ExportBytes int64         `json:"exportBytes"`
	ExportTotal int64         `json:"exportTotal"`
	OutputPath  string        `json:"outputPath,omitempty"`
	Error       string        `json:"error,omitempty"`
}

type pullProgress struct {
	mu sync.Mutex
	progressData
}

func (p *pullProgress) snapshot() progressData {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.progressData
}

func (p *pullProgress) setPhase(phase string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase = phase
}

func (p *pullProgress) setError(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.Contains(msg, "context canceled") {
		p.Phase = "error"
		p.Error = "download cancelled"
		return
	}
	p.Phase = "error"
	p.Error = msg
}

func (p *pullProgress) initLayers(n int, totalBytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.TotalLayers = n
	p.TotalBytes = totalBytes
	p.Layers = make([]layerStatus, n)
	for i := 0; i < n; i++ {
		p.Layers[i] = layerStatus{Index: i, Status: "waiting"}
	}
	p.Phase = "downloading"
}

func (p *pullProgress) updateLayer(idx int, evt puller.PullEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx < len(p.Layers) {
		p.Layers[idx].Digest = evt.Digest
		p.Layers[idx].Total = evt.Total
		if evt.Err != nil {
			p.Layers[idx].Status = "error"
			p.Phase = "error"
			errMsg := evt.Err.Error()
			if strings.Contains(errMsg, "context canceled") {
				errMsg = "download cancelled"
			}
			p.Error = errMsg
		} else {
			p.Layers[idx].Bytes = evt.Bytes
			p.Layers[idx].Status = evt.Status
		}
	}

	var doneBytes int64
	var doneLayers int
	for _, ls := range p.Layers {
		switch ls.Status {
		case "done", "cached":
			doneLayers++
			doneBytes += ls.Total
		case "downloading":
			doneBytes += ls.Bytes
		}
	}
	p.DoneLayers = doneLayers
	p.DoneBytes = doneBytes
}

func (p *pullProgress) setExport(completed, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ExportBytes = completed
	p.ExportTotal = total
}

func (p *pullProgress) setDone(outputPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase = "done"
	p.OutputPath = outputPath
}

var (
	guiProgressPtr atomic.Value
	downloadCancel context.CancelFunc
	cancelMu       sync.Mutex

	sseConnCount  int64
	shutdownTimer *time.Timer
	shutdownMu    sync.Mutex
)

func startShutdownTimer(d time.Duration) {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	cancelMu.Lock()
	active := downloadCancel != nil
	cancelMu.Unlock()
	if active {
		return
	}
	if shutdownTimer != nil {
		shutdownTimer.Stop()
	}
	shutdownTimer = time.AfterFunc(d, func() { os.Exit(0) })
}

func resetShutdownTimer() {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if shutdownTimer != nil {
		shutdownTimer.Stop()
		shutdownTimer = nil
	}
}

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Start web GUI (browser interface)",
	Long: `Start a web-based graphical user interface in your browser.

Opens a local HTTP server that provides an intuitive web interface
for downloading Docker images. Features:

  - Image name and platform selection
  - Real-time per-layer download progress
  - Mirror acceleration configuration (add/edit/remove)
  - Cache management (view size, clear)
  - Private registry authentication

Default: http://127.0.0.1:19191
Use --port to change the port.`,
	RunE: runGUI,
}

func init() {
	guiCmd.Flags().StringVarP(&guiPort, "port", "P", "19191", "Web GUI port (default: 19191)")
	guiCmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (default: OS-specific path)")
}

func runGUI(cmd *cobra.Command, args []string) error {
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser("http://127.0.0.1:" + guiPort)
	}()
	return serveGUI()
}

func StartGUI() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser("http://127.0.0.1:" + guiPort)
	}()
	err := serveGUI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "open browser: %v\n", err)
	}
}

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	os.Exit(0)
}

func handleOpenFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p := filepath.Clean(req.Path)
	if !filepath.IsAbs(p) || strings.Contains(p, "..") {
		http.Error(w, "invalid path", 400)
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", p)
	case "darwin":
		cmd = exec.Command("open", "-R", p)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(p))
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "open file: %v\n", err)
	}
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	cancelMu.Lock()
	if downloadCancel != nil {
		downloadCancel()
	}
	cancelMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func serveGUI() error {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("load web files: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/save", handleSave)
	mux.HandleFunc("/api/progress", handleProgress)
	mux.HandleFunc("/api/cache", handleCache)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/shutdown", handleShutdown)
	mux.HandleFunc("/api/cancel", handleCancel)
	mux.HandleFunc("/api/open-file", handleOpenFile)

	addr := "127.0.0.1:" + guiPort
	fmt.Printf("imgp GUI started at http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 400)
		return
	}

	var req struct {
		Image    string `json:"image"`
		Platform string `json:"platform"`
		Output   string `json:"output"`
		Username string `json:"username"`
		Password string `json:"password"`
		Insecure bool   `json:"insecure"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if req.Output != "" && (strings.Contains(req.Output, "..") || filepath.IsAbs(req.Output)) {
		http.Error(w, "invalid output path: must be a relative filename without '..'", 400)
		return
	}

	pp := &pullProgress{progressData: progressData{Phase: "starting"}}

	cancelMu.Lock()
	if downloadCancel != nil {
		downloadCancel()
	}
	cancelMu.Unlock()

	guiProgressPtr.Store(pp)
	resetShutdownTimer()

	go func() {
		cfg, err := config.Load()
		if err != nil {
			pp.setError(fmt.Sprintf("load config: %v", err))
			return
		}

		var cancel context.CancelFunc
		ctx := context.Background()
		if cfg.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, time.Duration(cfg.Timeout)*time.Minute)
		} else {
			ctx, cancel = context.WithCancel(ctx)
		}
		cancelMu.Lock()
		downloadCancel = cancel
		cancelMu.Unlock()
		defer cancel()

		client := registry.NewClient(cfg).WithAuth(req.Username, req.Password).WithInsecure(req.Insecure)

		plat := req.Platform
		if plat == "" {
			plat = "linux/amd64"
		}
		outPath := req.Output
		if outPath == "" {
			n := strings.ReplaceAll(req.Image, "/", "_")
			n = strings.ReplaceAll(n, ":", "_")
			p := strings.ReplaceAll(plat, "/", "-")
			outPath = fmt.Sprintf("%s_%s.tar", n, p)
		}

		img, ref, err := client.FetchImage(ctx, req.Image, plat)
		if err != nil {
			pp.setError(fmt.Sprintf("fetch image: %v", err))
			return
		}

		origRef, err := name.ParseReference(req.Image)
		if err != nil {
			pp.setError(fmt.Sprintf("parse image: %v", err))
			return
		}

		cd := cmdCacheDir()
		if err := os.MkdirAll(cd, 0755); err != nil {
			pp.setError(fmt.Sprintf("create cache: %v", err))
			return
		}

		layerFetcher := client.NewLayerFetcher(ref)
		imgLayers, err := img.Layers()
		if err != nil {
			pp.setError(fmt.Sprintf("get layers: %v", err))
			return
		}

		totalBytes := int64(0)
		tasks := make([]puller.LayerTask, len(imgLayers))
		for i, l := range imgLayers {
			digest, err := l.Digest()
			if err != nil {
				pp.setError(fmt.Sprintf("get layer %d digest: %v", i, err))
				return
			}
			size, err := l.Size()
			if err != nil {
				pp.setError(fmt.Sprintf("get layer %d size: %v", i, err))
				return
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
			totalBytes += size
		}

		pp.initLayers(len(tasks), totalBytes)

		p := cfg.Parallelism
		if p < 1 {
			p = 4
		}
		lt := cfg.LayerTimeout
		if lt == 0 {
			lt = 30
		}
		rt := cfg.Retry
		pl := puller.NewPuller(cd).
			WithLayerTimeout(time.Duration(lt)*time.Minute).
			WithRetry(rt)
		eventCh, err := pl.Pull(ctx, tasks, p)
		if err != nil {
			pp.setError(fmt.Sprintf("start pull: %v", err))
			return
		}

		for evt := range eventCh {
			pp.updateLayer(evt.Index, evt)
		}

		pp.setPhase("exporting")

		cachePathFn := func(digest string) string {
			return filepath.Join(cd, digest+".gz")
		}

		err = saver.Export(ctx, origRef, img, outPath, cachePathFn, func(completed, total int64) {
			pp.setExport(completed, total)
		})
		if err != nil {
			pp.setError(fmt.Sprintf("export: %v", err))
			return
		}

		pp.setDone(outPath)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&sseConnCount, 1)
	resetShutdownTimer()
	defer func() {
		if atomic.AddInt64(&sseConnCount, -1) == 0 {
			startShutdownTimer(5 * time.Second)
		}
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 400)
		return
	}

	for {
		ppv := guiProgressPtr.Load()
		if ppv == nil {
			select {
			case <-r.Context().Done():
				return
			default:
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}
		pp, ok := ppv.(*pullProgress)
		if !ok {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		s := pp.snapshot()
		data, err := json.Marshal(s)
		if err != nil {
			fmt.Fprintf(w, "data: {\"error\":%q}\n\n", err.Error())
		} else {
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		flusher.Flush()

		if s.Phase == "done" || s.Phase == "error" {
			break
		}

		for i := 0; i < 5; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
				time.Sleep(200 * time.Millisecond)
			}
		}
	}
}

func handleCache(w http.ResponseWriter, r *http.Request) {
	cd := cmdCacheDir()

	switch r.Method {
	case http.MethodGet:
		var totalSize int64
		var fileCount int
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".gz") {
					continue
				}
				fi, err := e.Info()
				if err == nil && !e.IsDir() {
					totalSize += fi.Size()
					fileCount++
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"path":  cd,
			"files": fileCount,
			"size":  totalSize,
		})

	case http.MethodPost:
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				if !strings.HasSuffix(e.Name(), ".gz") {
					continue
				}
				if err := os.RemoveAll(filepath.Join(cd, e.Name())); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		} else if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		} else {
			http.Error(w, err.Error(), 500)
		}

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		mirrorMap := make(map[string]string)
		for k, v := range cfg.MirrorMap {
			mirrorMap[k] = strings.Join(v, "|")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mirror_map":          mirrorMap,
			"insecure_registries": cfg.InsecureRegistries,
			"parallelism":         cfg.Parallelism,
			"layer_timeout":       cfg.LayerTimeout,
			"timeout":             cfg.Timeout,
			"retry":               cfg.Retry,
		})

	case http.MethodPost:
		var req struct {
			MirrorMap         map[string]string `json:"mirror_map"`
			InsecureRegistries []string         `json:"insecure_registries"`
			Parallelism       int              `json:"parallelism"`
			LayerTimeout      int              `json:"layer_timeout"`
			Timeout           int              `json:"timeout"`
			Retry             int              `json:"retry"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		if req.MirrorMap != nil {
			cfg.MirrorMap = make(map[string][]string)
			for k, v := range req.MirrorMap {
				if v == "" {
					continue
				}
				cfg.MirrorMap[k] = strings.Split(v, "|")
			}
		}
		if req.InsecureRegistries != nil {
			cfg.InsecureRegistries = req.InsecureRegistries
		}
		if req.Parallelism > 0 {
			if req.Parallelism > 64 {
				http.Error(w, "parallelism must be <= 64", 400)
				return
			}
			cfg.Parallelism = req.Parallelism
		}
		if req.LayerTimeout >= 0 {
			cfg.LayerTimeout = req.LayerTimeout
		}
		if req.Timeout >= 0 {
			cfg.Timeout = req.Timeout
		}
		if req.Retry >= 0 {
			cfg.Retry = req.Retry
		}

		if err := cfg.Save(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", 405)
	}
}
