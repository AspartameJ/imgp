package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/registry"
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
			if ui.IsStderrTerminal() {
				fmt.Fprint(os.Stderr, "\033[J")
			}
			fmt.Fprintln(os.Stderr)
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
	start := time.Now()
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

	mr, err := fetchManifest(ctx, client, image, p.targetPlatform, batchInfo, p.quiet)
	if err != nil {
		return err
	}

	if err := pullLayers(ctx, cd, p, mr.img, mr.ref, client); err != nil {
		return err
	}

	return exportImage(ctx, cd, p, mr.img, mr.origRef, image, start)
}
