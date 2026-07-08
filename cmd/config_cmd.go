package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gitcode.com/DonaldTom/imgp/internal/config"
)

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
  retry               Number of retries on network errors (default: 2)
  cache-dir           Custom cache directory path (default: OS-specific path)`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := setConfigKey(cfg, args[0], args[1]); err != nil {
			return err
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
		data += fmt.Sprintf("Cache Dir: %s\n", cfg.CacheDir)
		_, err = fmt.Print(data)
		return err
	},
}

func setConfigKey(cfg *config.Config, key, value string) error {
	switch key {
	case "mirror-map":
		if cfg.MirrorMap == nil {
			cfg.MirrorMap = make(map[string][]string)
		}
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
	case "cache-dir":
		cfg.CacheDir = value
	default:
		return fmt.Errorf("unknown key: %s (supported: mirror-map, insecure-registries, parallelism, layer-timeout, timeout, retry, cache-dir)", key)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
}
