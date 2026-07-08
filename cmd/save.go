package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/puller"
	"gitcode.com/DonaldTom/imgp/internal/registry"
	"gitcode.com/DonaldTom/imgp/internal/saver"
	"gitcode.com/DonaldTom/imgp/internal/ui"
)

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
			if ui.IsTerminal() {
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
	p, err := resolveSaveParams(cmd, cfg, image)
	if err != nil {
		return err
	}

	cd := cmdCacheDir(cfg)
	if err := os.MkdirAll(cd, 0755); err != nil {
		return fmt.Errorf("create cache: %w", err)
	}

	client := registry.NewClient(cfg).WithAuth(username, p.password).WithInsecure(insecure).WithRetry(p.retry)

	ctx := cmd.Context()
	if p.overallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.overallTimeout)*time.Minute)
		defer cancel()
	}

	mr, err := fetchManifest(ctx, cfg, client, image, p.targetPlatform, batchInfo, p.quiet)
	if err != nil {
		return err
	}

	if err := pullLayers(ctx, cd, p, mr.img, mr.ref, client); err != nil {
		return err
	}

	return exportImage(ctx, cd, p, mr.img, mr.origRef, image)
}

type manifestResult struct {
	img     v1.Image
	ref     name.Reference
	origRef name.Reference
}

func fetchManifest(ctx context.Context, cfg *config.Config, client *registry.Client, image, targetPlatform, batchInfo string, quiet bool) (*manifestResult, error) {
	if !quiet {
		if ui.IsTerminal() {
			fmt.Printf("%s%s %s (%s)\n", batchInfo, ui.Cyan("⟳ Pulling"), image, targetPlatform)
		} else {
			fmt.Printf("%sPulling %s (%s)\n", batchInfo, image, targetPlatform)
		}
	}

	img, ref, err := client.FetchImage(ctx, image, targetPlatform)
	if err != nil {
		if isNetworkError(err) {
			return nil, fmt.Errorf("fetch image: %w\n       tip: check network/proxy or try with a mirror (e.g. `imgp config set mirror-map registry.k8s.io=m.daocloud.io/registry.k8s.io`)", err)
		}
		return nil, fmt.Errorf("fetch image: %w", err)
	}

	origRef, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}

	if !quiet {
		if ui.IsTerminal() {
			fmt.Printf("%s\n", ui.Green("✓ Image manifest fetched, downloading layers..."))
		} else {
			fmt.Println("Image manifest fetched, downloading layers...")
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

func exportImage(ctx context.Context, cd string, p saveParams, img v1.Image, origRef name.Reference, image string) error {
	if !p.quiet {
		if ui.IsTerminal() {
			fmt.Printf("\n%s\n", ui.Cyan("⟳ Exporting to ")+p.outputPath)
		} else {
			fmt.Printf("\nExporting to %s\n", p.outputPath)
		}
	}

	cachePathFn := func(digest string) string {
		return filepath.Join(cd, digest+".gz")
	}

	if !p.quiet {
		fmt.Printf("\r  exporting: 0%%")
	}
	err := saver.Export(ctx, origRef, img, p.outputPath, cachePathFn, p.gzip,
		func(completed, total int64) {
			if p.quiet {
				return
			}
			percent := float64(completed) / float64(total) * 100
			fmt.Printf("\r  exporting: %.0f%% | %s / %s",
				percent, ui.FormatBytes(completed), ui.FormatBytes(total))
		},
	)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if !p.quiet {
		if ui.IsTerminal() {
			fmt.Printf("\n%s %s (%s) saved to %s\n", ui.Green("✓ Done:"), image, p.targetPlatform, p.outputPath)
		} else {
			fmt.Printf("\nDone: %s (%s) saved to %s\n", image, p.targetPlatform, p.outputPath)
		}
	} else {
		fmt.Println(p.outputPath)
	}
	return nil
}

func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "dial tcp") || strings.Contains(msg, "dial tcp: lookup") || strings.Contains(msg, "i/o timeout")
}
