package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
)

func TestE2E_FullPipeline(t *testing.T) {
	tests := []struct {
		name   string
		gzip   bool
		resume bool
		check  func(t *testing.T, data []byte)
	}{
		{
			name: "tar",
			gzip: false,
			check: func(t *testing.T, data []byte) {
				if len(data) == 0 {
					t.Error("exported tar is empty")
				}
			},
		},
		{
			name: "gzip",
			gzip: true,
			check: func(t *testing.T, data []byte) {
				if len(data) < 20 {
					t.Error("exported gzip tar is too small")
				}
				if data[0] != 0x1f || data[1] != 0x8b {
					t.Error("output is not a valid gzip file (missing gzip magic bytes)")
				}
			},
		},
		{
			name:   "resume",
			resume: true,
			check: func(t *testing.T, data []byte) {
				if len(data) == 0 {
					t.Error("exported tar is empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _, _, refStr := mockRegistryServer(t, 1024, 2)
			defer srv.Close()

			cfg := config.DefaultConfig()
			cfg.MirrorMap = nil

			client := registry.NewClient(cfg).WithRetry(0)
			ctx := context.Background()
			fetchedImg, ref, err := client.FetchImage(ctx, refStr, "linux/amd64")
			if err != nil {
				t.Fatalf("FetchImage: %v", err)
			}

			cacheDir, err := os.MkdirTemp("", "imgp-e2e-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { runtime.GC(); os.RemoveAll(cacheDir) }()

			layerFetcher := client.NewLayerFetcher(ref)
			imgLayers, err := fetchedImg.Layers()
			if err != nil {
				t.Fatal(err)
			}

			tasks := make([]puller.LayerTask, len(imgLayers))
			for i, l := range imgLayers {
				d, err := l.Digest()
				if err != nil {
					t.Fatal(err)
				}
				size, err := l.Size()
				if err != nil {
					t.Fatal(err)
				}
				dHex := d.Hex
				tasks[i] = puller.LayerTask{
					Index:     i,
					DigestHex: dHex,
					Size:      size,
					OpenLayer: func(ctx context.Context, offset int64) (io.ReadCloser, error) {
						return layerFetcher(ctx, dHex, offset)
					},
				}
			}

			pl := puller.NewPuller(cacheDir).WithResume(tt.resume)
			eventCh, err := pl.Pull(ctx, tasks, 1)
			if err != nil {
				t.Fatalf("Pull: %v", err)
			}
			for evt := range eventCh {
				if evt.Err != nil {
					t.Fatalf("layer %d pull error: %v", evt.Index, evt.Err)
				}
			}

			outName := "out.tar"
			if tt.gzip {
				outName = "out.tar.gz"
			}
			outPath := filepath.Join(cacheDir, outName)
			cachePathFn := func(digest string) string {
				return filepath.Join(cacheDir, digest+".gz")
			}

			err = saver.Export(ctx, ref, fetchedImg, outPath, cachePathFn, tt.gzip, func(completed, total int64) {})
			if err != nil {
				t.Fatalf("Export: %v", err)
			}

			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, data)
		})
	}
}
