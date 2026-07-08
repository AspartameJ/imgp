package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
	"gitcode.com/DonaldTom/imgp/internal/ui"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage layer cache",
	Long:  "View and manage the layer cache to avoid re-downloading layers on repeat pulls.",
}

func cacheInfo(cfg *config.Config) (path string, fileCount int, totalSize int64) {
	cd := cmdCacheDir(cfg)
	entries, err := os.ReadDir(cd)
	if err != nil {
		return cd, 0, 0
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
	return cd, fileCount, totalSize
}

func cacheClear(cfg *config.Config) (removed int, freed int64) {
	cd := cmdCacheDir(cfg)
	if entries, err := os.ReadDir(cd); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".gz") {
				continue
			}
			if e.IsDir() {
				continue
			}
			if err := os.Remove(filepath.Join(cd, e.Name())); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "cache clear: remove %s: %v\n", e.Name(), err)
				continue
			}
			if fi, err := e.Info(); err == nil {
				freed += fi.Size()
			}
			removed++
		}
	}
	return removed, freed
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache usage",
	Long:  "Display the cache directory path, number of cached layers, and total disk usage.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cd, fileCount, totalSize := cacheInfo(cfg)
		if _, err := os.Stat(cd); os.IsNotExist(err) {
			fmt.Printf("Cache directory: %s (not yet created)\n", cd)
			fmt.Printf("Cached layers:   0\n")
			fmt.Printf("Total size:      0 B\n")
		} else {
			fmt.Printf("Cache directory: %s\n", cd)
			fmt.Printf("Cached layers:   %d\n", fileCount)
			fmt.Printf("Total size:      %s\n", ui.FormatBytes(totalSize))
		}
		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached layers",
	Long:  "Remove all cached layer files to free up disk space.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		removed, freed := cacheClear(cfg)
		fmt.Printf("Cleared %d cached layers (%s)\n", removed, ui.FormatBytes(freed))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheInfoCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "Custom cache directory (default: OS-specific path)")
}
