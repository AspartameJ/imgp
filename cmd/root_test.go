package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/registry"
)

func TestResolveSaveParams_Defaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cmd := &cobra.Command{}

	p, err := resolveSaveParams(cmd, cfg, "myimg:latest")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.parallelism != 4 {
		t.Errorf("parallelism = %d, want 4", p.parallelism)
	}
	if p.layerTimeout != 30 {
		t.Errorf("layerTimeout = %d, want 30", p.layerTimeout)
	}
	if p.retry != 2 {
		t.Errorf("retry = %d, want 2", p.retry)
	}
	if p.targetPlatform != "linux/amd64" {
		t.Errorf("platform = %q, want linux/amd64", p.targetPlatform)
	}
	if p.outputPath != "myimg_latest_linux-amd64.tar" {
		t.Errorf("outputPath = %q, want myimg_latest_linux-amd64.tar", p.outputPath)
	}
	if p.gzip {
		t.Error("gzip should be false by default")
	}
	if p.noCache {
		t.Error("noCache should be false by default")
	}
}

func TestResolveSaveParams_ParallelismFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Parallelism = 8
	cmd := &cobra.Command{}

	p, err := resolveSaveParams(cmd, cfg, "test:latest")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.parallelism != 8 {
		t.Errorf("parallelism = %d, want 8", p.parallelism)
	}
}

func TestResolveSaveParams_InvalidPlatform(t *testing.T) {
	cfg := config.DefaultConfig()
	platform = "bad"
	defer func() { platform = "" }()

	cmd := &cobra.Command{}
	_, err := resolveSaveParams(cmd, cfg, "test:latest")
	if err == nil {
		t.Fatal("expected error for invalid platform")
	}
}

func TestResolveSaveParams_GzipOutput(t *testing.T) {
	cfg := config.DefaultConfig()
	gzip = true
	defer func() { gzip = false }()

	cmd := &cobra.Command{}
	p, err := resolveSaveParams(cmd, cfg, "myimg:latest")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if !p.gzip {
		t.Error("gzip should be true")
	}
	if p.outputPath != "myimg_latest_linux-amd64.tar.gz" {
		t.Errorf("outputPath = %q, want .tar.gz", p.outputPath)
	}
}

func TestResolveSaveParams_CustomOutput(t *testing.T) {
	cfg := config.DefaultConfig()
	output = "test-out.tar"
	defer func() { output = "" }()

	cmd := &cobra.Command{}
	p, err := resolveSaveParams(cmd, cfg, "myimg:latest")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.outputPath != "test-out.tar" {
		t.Errorf("outputPath = %q, want test-out.tar", p.outputPath)
	}
}

func TestResolveSaveParams_TimeoutFromConfig(t *testing.T) {
	saveFlag := timeoutMin
	timeoutMin = 0
	defer func() { timeoutMin = saveFlag }()

	cfg := config.DefaultConfig()
	cfg.Timeout = 15

	cmd := &cobra.Command{}
	p, err := resolveSaveParams(cmd, cfg, "img:tag")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.overallTimeout != 15 {
		t.Errorf("overallTimeout = %d, want 15 (from config)", p.overallTimeout)
	}
}

func TestResolveSaveParams_RetryFromFlag(t *testing.T) {
	saveFlag := retryCount
	retryCount = 5
	defer func() { retryCount = saveFlag }()

	cfg := config.DefaultConfig()
	cmd := &cobra.Command{}
	cmd.Flags().Int("retry", 2, "")
	cmd.Flags().Set("retry", "5")

	p, err := resolveSaveParams(cmd, cfg, "img:tag")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.retry != 5 {
		t.Errorf("retry = %d, want 5", p.retry)
	}
}

func TestResolveSaveParams_RetryClamp(t *testing.T) {
	saveFlag := retryCount
	retryCount = 50
	defer func() { retryCount = saveFlag }()

	cfg := config.DefaultConfig()
	cmd := &cobra.Command{}
	cmd.Flags().Int("retry", 2, "")
	cmd.Flags().Set("retry", "50")

	p, err := resolveSaveParams(cmd, cfg, "img:tag")
	if err != nil {
		t.Fatalf("resolveSaveParams error = %v", err)
	}
	if p.retry != 30 {
		t.Errorf("retry = %d, want 30 (clamped)", p.retry)
	}
}

func TestRunSaveOne(t *testing.T) {
	tests := []struct {
		name string
		gzip bool
	}{
		{"tar", false},
		{"gzip", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer saveGlobals()()

			srv, _, _, refStr := mockRegistryServer(t, 1024, 2)
			defer srv.Close()

			baseDir, err := os.MkdirTemp("", "imgp-test-save-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { runtime.GC(); os.RemoveAll(baseDir) }()

			ext := ".tar"
			if tt.gzip {
				ext = ".tar.gz"
			}
			outPath := filepath.Join(baseDir, "out"+ext)

			output = outPath
			platform = ""
			quiet = true
			gzip = tt.gzip
			noCache = false
			cacheDir = baseDir
			username = ""
			insecure = false
			parallelism = 4
			retryCount = 2
			timeoutMin = 0
			layerTimeoutMin = 30
			password = ""
			passwordEnv = ""

			cfg := config.DefaultConfig()
			cfg.MirrorMap = nil
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())

			err = runSaveOne(cmd, cfg, refStr, "")
			if err != nil {
				t.Fatalf("runSaveOne error = %v", err)
			}

			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) == 0 {
				t.Error("output is empty")
			}
			if tt.gzip && (data[0] != 0x1f || data[1] != 0x8b) {
				t.Error("output is not a valid gzip file")
			}
		})
	}
}

func TestRunSave_Batch_RejectsOutput(t *testing.T) {
	saveOutput := output
	defer func() { output = saveOutput }()
	output = "out.tar"

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runSave(cmd, []string{"img1:latest", "img2:latest"})
	if err == nil {
		t.Fatal("expected error for -o with multiple images")
	}
	if !strings.Contains(err.Error(), "cannot use -o") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSave_Batch_ErrorAggregation(t *testing.T) {
	saveCacheDir := cacheDir
	saveQuiet := quiet
	defer func() {
		cacheDir = saveCacheDir
		quiet = saveQuiet
	}()

	baseDir, err := os.MkdirTemp("", "imgp-test-batch-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	cacheDir = baseDir
	quiet = true

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err = runSave(cmd, []string{"!!!invalid!!!", "!!!also-bad!!!"})
	if err == nil {
		t.Fatal("expected error for invalid image names")
	}
	msg := err.Error()
	if !strings.Contains(msg, "batch download completed with errors") {
		t.Errorf("unexpected error header: %v", err)
	}
}

func TestRunSave_Batch_ContextCancel(t *testing.T) {
	saveCacheDir := cacheDir
	saveQuiet := quiet
	defer func() {
		cacheDir = saveCacheDir
		quiet = saveQuiet
	}()

	baseDir, err := os.MkdirTemp("", "imgp-test-batch-cancel-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	cacheDir = baseDir
	quiet = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	err = runSave(cmd, []string{"img1:latest", "img2:latest"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	msg := err.Error()
	if !strings.Contains(msg, "canceled by user") {
		t.Errorf("expected cancellation message, got: %v", err)
	}
}

func TestResolvePlatform(t *testing.T) {
	t.Run("default empty returns linux/amd64", func(t *testing.T) {
		p, err := resolvePlatform("")
		if err != nil {
			t.Fatal(err)
		}
		if p != "linux/amd64" {
			t.Errorf("got %q", p)
		}
	})

	t.Run("too many parts rejected", func(t *testing.T) {
		_, err := resolvePlatform("a/b/c/d")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty segment rejected", func(t *testing.T) {
		_, err := resolvePlatform("/amd64")
		if err == nil {
			t.Fatal("expected error for empty segment")
		}
		_, err = resolvePlatform("linux//v8")
		if err == nil {
			t.Fatal("expected error for empty segment")
		}
	})

	t.Run("valid 2-part platform", func(t *testing.T) {
		p, err := resolvePlatform("linux/arm64")
		if err != nil {
			t.Fatal(err)
		}
		if p != "linux/arm64" {
			t.Errorf("got %q", p)
		}
	})

	t.Run("valid 3-part platform", func(t *testing.T) {
		p, err := resolvePlatform("linux/arm64/v8")
		if err != nil {
			t.Fatal(err)
		}
		if p != "linux/arm64/v8" {
			t.Errorf("got %q", p)
		}
	})

	t.Run("unknown arch without suggestion", func(t *testing.T) {
		_, err := resolvePlatform("linux/unknown-arch-12345")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown arch with suggestion", func(t *testing.T) {
		_, err := resolvePlatform("linux/amd64_") // close to amd64
		if err == nil {
			t.Fatal("expected error for unknown arch")
		}
	})
}

func TestResolvePassword(t *testing.T) {
	t.Run("direct password", func(t *testing.T) {
		saved := password
		password = "direct-pass"
		defer func() { password = saved }()
		if got := resolvePassword(); got != "direct-pass" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("env var password", func(t *testing.T) {
		savedPW := password
		savedEnv := passwordEnv
		password = ""
		passwordEnv = "TEST_IMGP_PASS"
		defer func() {
			password = savedPW
			passwordEnv = savedEnv
		}()
		os.Setenv("TEST_IMGP_PASS", "env-pass")
		defer os.Unsetenv("TEST_IMGP_PASS")
		if got := resolvePassword(); got != "env-pass" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty fallback", func(t *testing.T) {
		savedPW := password
		savedEnv := passwordEnv
		password = ""
		passwordEnv = ""
		defer func() {
			password = savedPW
			passwordEnv = savedEnv
		}()
		if got := resolvePassword(); got != "" {
			t.Errorf("got %q", got)
		}
	})
}

func TestCmdCacheDir(t *testing.T) {
	t.Run("flag set uses flag", func(t *testing.T) {
		saved := cacheDir
		cacheDir = "/custom/cache"
		defer func() { cacheDir = saved }()
		cfg := config.DefaultConfig()
		if got := cmdCacheDir(cfg); got != "/custom/cache" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("empty flag uses config", func(t *testing.T) {
		saved := cacheDir
		cacheDir = ""
		defer func() { cacheDir = saved }()
		cfg := &config.Config{CacheDir: "/cfg/cache"}
		if got := cmdCacheDir(cfg); got != "/cfg/cache" {
			t.Errorf("got %q", got)
		}
	})
}

func TestIsNetworkError(t *testing.T) {
	t.Run("timeout error", func(t *testing.T) {
		err := &timeoutErr{}
		if !isNetworkError(err) {
			t.Error("expected true for timeout")
		}
	})

	t.Run("dial tcp error", func(t *testing.T) {
		err := fmt.Errorf("dial tcp 1.2.3.4:80: connectex: connection refused")
		if !isNetworkError(err) {
			t.Error("expected true for dial tcp")
		}
	})

	t.Run("dial tcp lookup error", func(t *testing.T) {
		err := fmt.Errorf("dial tcp: lookup nonexistent.example.com: no such host")
		if !isNetworkError(err) {
			t.Error("expected true for lookup error")
		}
	})

	t.Run("i/o timeout string", func(t *testing.T) {
		err := fmt.Errorf("something: i/o timeout")
		if !isNetworkError(err) {
			t.Error("expected true for i/o timeout")
		}
	})

	t.Run("non-network error", func(t *testing.T) {
		err := fmt.Errorf("random error")
		if isNetworkError(err) {
			t.Error("expected false for random error")
		}
	})
}

// timeoutErr implements net.Error with Timeout()=true
type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return true }

func TestFetchManifest_Display(t *testing.T) {
	srv, _, _, refStr := mockRegistryServer(t, 256, 1)
	defer srv.Close()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := config.DefaultConfig()
	client := registry.NewClient(cfg).WithInsecure(true)
	cfg.MirrorMap = nil
	mr, err := fetchManifest(context.Background(), cfg, client, refStr, "linux/amd64", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if mr.img == nil {
		t.Error("expected non-nil image")
	}

	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	r.Close()

	if !strings.Contains(string(out), "Pulling") {
		t.Errorf("expected 'Pulling' in output, got: %s", string(out))
	}
}

func TestFetchManifest_NonNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	srvURL, _ := url.Parse(srv.URL)
	refStr := fmt.Sprintf("localhost:%s/testimg:err", srvURL.Port())

	cfg := config.DefaultConfig()
	client := registry.NewClient(cfg).WithInsecure(true)
	cfg.MirrorMap = nil
	_, err := fetchManifest(context.Background(), cfg, client, refStr, "linux/amd64", "", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "tip: check network") {
		t.Error("should not show network tip for non-network error")
	}
}

func TestPullLayers_ErrorReport(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "imgp-pull-err-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()

	srv, _, _, refStr := mockRegistryServer(t, 256, 1)
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	client := registry.NewClient(cfg).WithRetry(0)
	ctx := context.Background()

	fetchedImg, ref, err := client.FetchImage(ctx, refStr, "linux/amd64")
	if err != nil {
		t.Fatalf("FetchImage should succeed: %v", err)
	}

	// Shut down mock server so layer downloads fail
	srv.Close()

	p := saveParams{
		parallelism: 1, retry: 0, noCache: true,
		layerTimeout: 1, quiet: true,
	}
	err = pullLayers(ctx, baseDir, p, fetchedImg, ref, client)
	if err == nil {
		t.Fatal("expected pullLayers error after server shutdown")
	}
	if !strings.Contains(err.Error(), "layer") {
		t.Errorf("pullLayers error should mention layer, got: %v", err)
	}
}

func TestExportImage_Error(t *testing.T) {
	img, err := random.Image(256, 1)
	if err != nil {
		t.Fatal(err)
	}
	origRef, err := name.ParseReference("testimg:exportbad")
	if err != nil {
		t.Fatal(err)
	}

	// Write to a path that will fail — a non-existent directory
	err = exportImage(context.Background(), "/nonexistent-parent-dir/imgp-test", saveParams{
		outputPath:     "/nonexistent-parent-dir/imgp-test/out.tar",
		quiet:          true,
		gzip:           false,
		targetPlatform: "linux/amd64",
	}, img, origRef, "testimg:exportbad")
	if err == nil {
		t.Fatal("expected error for invalid output path")
	}
}

func TestRunSaveOne_FetchError(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "imgp-test-err-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()
	outPath := filepath.Join(baseDir, "out.tar")

	saveOutput := output
	saveQuiet := quiet
	saveCacheDir := cacheDir
	saveParallelism := parallelism
	defer func() {
		output = saveOutput
		quiet = saveQuiet
		cacheDir = saveCacheDir
		parallelism = saveParallelism
	}()

	output = outPath
	quiet = true
	cacheDir = baseDir
	parallelism = 4

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err = runSaveOne(cmd, cfg, "nonexistent.registry.invalid/img:nonexistent", "")
	if err == nil {
		t.Fatal("expected error for nonexistent registry")
	}
}

func TestRunSaveOne_Timeout(t *testing.T) {
	saveTimeout := timeoutMin
	timeoutMin = 5
	defer func() { timeoutMin = saveTimeout }()

	baseDir, err := os.MkdirTemp("", "imgp-test-timeout-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { runtime.GC(); os.RemoveAll(baseDir) }()
	outPath := filepath.Join(baseDir, "out.tar")

	saveOutput := output
	saveQuiet := quiet
	saveCacheDir := cacheDir
	saveParallelism := parallelism
	defer func() {
		output = saveOutput
		quiet = saveQuiet
		cacheDir = saveCacheDir
		parallelism = saveParallelism
	}()

	output = outPath
	quiet = true
	cacheDir = baseDir
	parallelism = 4

	cfg := config.DefaultConfig()
	cfg.MirrorMap = nil
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err = runSaveOne(cmd, cfg, "nonexistent.registry.invalid/img:nonexistent", "")
	if err == nil {
		t.Fatal("expected error for nonexistent registry with timeout")
	}
}
