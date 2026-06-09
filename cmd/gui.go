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
	"time"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
)

//go:embed web
var webFS embed.FS

var guiPort string

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
			p.Error = evt.Err.Error()
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

var guiProgress pullProgress

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

Default: http://127.0.0.1:8080
Use --port to change the port.`,
	RunE: runGUI,
}

func init() {
	guiCmd.Flags().StringVarP(&guiPort, "port", "P", "8080", "Web GUI port (default: 8080)")
}

func runGUI(cmd *cobra.Command, args []string) error {
	openBrowser("http://127.0.0.1:" + guiPort)
	return serveGUI()
}

func StartGUI() {
	openBrowser("http://127.0.0.1:" + guiPort)
	err := serveGUI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "GUI error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
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

	guiProgress = pullProgress{progressData: progressData{Phase: "starting"}}

	go func() {
		cfg, err := config.Load()
		if err != nil {
			guiProgress.setError(fmt.Sprintf("load config: %v", err))
			return
		}

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

		img, ref, err := client.FetchImage(r.Context(), req.Image, plat)
		if err != nil {
			guiProgress.setError(fmt.Sprintf("fetch image: %v", err))
			return
		}

		cd := cmdCacheDir()
		if err := os.MkdirAll(cd, 0755); err != nil {
			guiProgress.setError(fmt.Sprintf("create cache: %v", err))
			return
		}

		layerFetcher := client.NewLayerFetcher(ref)
		imgLayers, err := img.Layers()
		if err != nil {
			guiProgress.setError(fmt.Sprintf("get layers: %v", err))
			return
		}

		totalBytes := int64(0)
		tasks := make([]puller.LayerTask, len(imgLayers))
		for i, l := range imgLayers {
			digest, _ := l.Digest()
			size, _ := l.Size()
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

		guiProgress.initLayers(len(tasks), totalBytes)

		pl := puller.NewPuller(cd)
		eventCh, err := pl.Pull(r.Context(), tasks, 4)
		if err != nil {
			guiProgress.setError(fmt.Sprintf("start pull: %v", err))
			return
		}

		for evt := range eventCh {
			guiProgress.updateLayer(evt.Index, evt)
			if evt.Err != nil {
				return
			}
		}

		guiProgress.setPhase("exporting")

		cachePathFn := func(digest string) string {
			return filepath.Join(cd, digest+".gz")
		}

		err = saver.Export(ref, img, outPath, cachePathFn, func(completed, total int64) {
			guiProgress.setExport(completed, total)
		})
		if err != nil {
			guiProgress.setError(fmt.Sprintf("export: %v", err))
			return
		}

		guiProgress.setDone(outPath)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 400)
		return
	}

	for {
		s := guiProgress.snapshot()
		data, _ := json.Marshal(s)
		fmt.Fprintf(w, "data: %s\n\n", data)
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
	cd := config.CacheDir()

	switch r.Method {
	case http.MethodGet:
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"path":  cd,
			"files": fileCount,
			"size":  totalSize,
		})

	case http.MethodPost:
		if entries, err := os.ReadDir(cd); err == nil {
			for _, e := range entries {
				os.RemoveAll(filepath.Join(cd, e.Name()))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mirror_map":          cfg.MirrorMap,
			"insecure_registries": cfg.InsecureRegistries,
			"parallelism":         cfg.Parallelism,
		})

	case http.MethodPost:
		var req struct {
			MirrorMap         map[string]string `json:"mirror_map"`
			InsecureRegistries []string         `json:"insecure_registries"`
			Parallelism       int              `json:"parallelism"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		if req.MirrorMap != nil {
			cfg.MirrorMap = make(map[string][]string)
			for k, v := range req.MirrorMap {
				cfg.MirrorMap[k] = []string{v}
			}
		}
		if req.InsecureRegistries != nil {
			cfg.InsecureRegistries = req.InsecureRegistries
		}
		if req.Parallelism > 0 {
			cfg.Parallelism = req.Parallelism
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
