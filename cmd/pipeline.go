package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
	"gitcode.com/DonaldTom/imgp/internal/ui"
	"gitcode.com/DonaldTom/imgp/internal/util"
)

type manifestResult struct {
	img     v1.Image
	ref     name.Reference
	origRef name.Reference
}

func fetchManifest(ctx context.Context, client *registry.Client, image, targetPlatform, batchInfo string, quiet bool) (*manifestResult, error) {
	if !quiet {
		if ui.IsStderrTerminal() {
			fmt.Fprintf(os.Stderr, "%s%s %s (%s)\n", batchInfo, ui.Cyan("⟳ Pulling"), image, targetPlatform)
		} else {
			fmt.Fprintf(os.Stderr, "%sPulling %s (%s)\n", batchInfo, image, targetPlatform)
		}
	}

	img, ref, err := client.FetchImage(ctx, image, targetPlatform)
	if err != nil {
		if util.IsConnectivityError(err) {
			return nil, fmt.Errorf("fetch image: %w\n       tip: check network/proxy or try with a mirror (e.g. `imgp config set mirror-map registry.k8s.io=m.daocloud.io/registry.k8s.io`)", err)
		}
		return nil, fmt.Errorf("fetch image: %w", err)
	}

	origRef, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}

	if !quiet {
		if ui.IsStderrTerminal() {
			fmt.Fprintf(os.Stderr, "%s\n", ui.Green("✓ Image manifest fetched, downloading layers..."))
		} else {
			fmt.Fprintln(os.Stderr, "Image manifest fetched, downloading layers...")
		}
	}

	return &manifestResult{img: img, ref: ref, origRef: origRef}, nil
}

func pullLayers(ctx context.Context, cd string, p saveParams, img v1.Image, ref name.Reference, client *registry.Client) error {
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

	pl := puller.NewPuller(cd).WithNoCache(p.noCache).WithLayerTimeout(time.Duration(p.layerTimeout) * time.Minute).WithRetry(p.retry)

	eventCh, err := pl.Pull(ctx, tasks, p.parallelism)
	if err != nil {
		return fmt.Errorf("start pull: %w", err)
	}

	progress := ui.NewProgressDisplay(p.quiet)
	pullDone := progress.RunPullUI(ctx, eventCh, tasks)
	<-pullDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if progress.GetHasError() {
		var errs []string
		for _, ls := range progress.GetLayers() {
			if ls.Status == "error" {
				msg := ls.ErrMsg
				if msg == "" {
					msg = "unknown error"
				}
				errs = append(errs, fmt.Sprintf("layer %s: %s", ui.Shorten(ls.Digest, 12), msg))
			}
		}
		return fmt.Errorf("layer download failed:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

func exportImage(ctx context.Context, cd string, p saveParams, img v1.Image, origRef name.Reference, image string, start time.Time) error {
	elapsed := time.Since(start).Round(time.Millisecond * 100)
	if !p.quiet {
		if ui.IsStderrTerminal() {
			fmt.Fprintf(os.Stderr, "\n%s\n", ui.Cyan("⟳ Exporting to ")+p.outputPath)
		} else {
			fmt.Fprintf(os.Stderr, "\nExporting to %s\n", p.outputPath)
		}
	}

	cachePathFn := func(digest string) string {
		return filepath.Join(cd, digest+".gz")
	}

	if !p.quiet {
		fmt.Fprintf(os.Stderr, "\r  exporting: 0%%")
	}
	err := saver.Export(ctx, origRef, img, p.outputPath, cachePathFn, p.gzip,
		func(completed, total int64) {
			if p.quiet {
				return
			}
			percent := 0.0
			if total > 0 {
				percent = float64(completed) / float64(total) * 100
			}
			fmt.Fprintf(os.Stderr, "\r  exporting: %.0f%% | %s / %s",
				percent, ui.FormatBytes(completed), ui.FormatBytes(total))
		},
	)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if !p.quiet {
		if ui.IsTerminal() {
			fmt.Printf("\n%s %s (%s) saved to %s (%v)\n", ui.Green("✓ Done:"), image, p.targetPlatform, p.outputPath, elapsed)
		} else {
			fmt.Printf("\nDone: %s (%s) saved to %s (%v)\n", image, p.targetPlatform, p.outputPath, elapsed)
		}
	} else {
		fmt.Println(p.outputPath)
	}
	return nil
}
