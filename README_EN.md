English | [中文](README.md)

# imgp

**imgp** is a cross-platform Docker image pull and save tool. It pulls images from registries (Docker Hub, quay.io, gcr.io, etc.) and exports them as standard `.tar` files that can be imported with `docker load` — no Docker daemon required.

## Common Use Cases

| Scenario | How |
|---|---|
| Download an image on a machine without Docker | `imgp save nginx:latest -o nginx.tar`, then `docker load` on the target machine |
| Docker Hub is slow in your region | Built-in mirror acceleration routes through fast mirrors automatically |
| Need an arm64 image for Raspberry Pi | `imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar` |
| Private registry requires login | `imgp save private/app:latest --username user --password-env` |
| Download interrupted halfway | Cached layers resume automatically — no re-download needed |

## Install

### Option 1: Download binary (recommended)

Download from the [Releases page](https://gitcode.com/DonaldTom/imgp/releases):

| Platform | File |
|---|---|
| Windows (64-bit) | `imgp-windows-amd64.exe` |
| Windows (ARM) | `imgp-windows-arm64.exe` |
| Linux (64-bit) | `imgp-linux-amd64` |
| Linux (ARM64) | `imgp-linux-arm64` |
| macOS (Intel) | `imgp-darwin-amd64` |
| macOS (Apple Silicon) | `imgp-darwin-arm64` |

**Linux / macOS**:

```bash
chmod +x imgp-linux-amd64
sudo mv imgp-linux-amd64 /usr/local/bin/imgp
```

**Windows**: Place the `.exe` in any directory, or better, in a directory listed in your `PATH` environment variable (e.g., `C:\Users\yourname\go\bin\`).

### Option 2: Install via Go

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

> For users in China who encounter network issues, set Go proxy:
> ```bash
> go env -w GOPROXY=https://goproxy.cn,direct
> ```

### Verify

```bash
imgp version
```

## Quick Start

```bash
# Pull and save hello-world (a tiny test image)
imgp save hello-world:latest -o hello-world.tar

# Load into Docker to verify
docker load -i hello-world.tar
```

### Specify a platform

Pull the arm64 version:

```bash
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar
```

### Private registry

```bash
# Password from environment variable (recommended)
export IMG_REGISTRY_PASSWORD=your_password
imgp save private.registry.com/myapp:latest --username user

# Direct password (not recommended — visible in process listings)
imgp save private.registry.com/myapp:latest --username user --password your_password
```

## Commands

### `imgp save [image]` — Pull and export

```bash
imgp save [image] [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-o, --output` | `image_platform.tar` | Output tar file path |
| `-p, --platform` | `linux/amd64` | Target platform (e.g. `linux/arm64`, `windows/amd64`) |
| `--username` | (empty) | Registry username |
| `--password` | (empty) | Registry password (use `--password-env`) |
| `--password-env` | `IMG_REGISTRY_PASSWORD` | Env var name for password |
| `--insecure` | false | Allow non-TLS connections |
| `-P, --parallel` | 4 (from config) | Number of parallel layer downloads |
| `--no-cache` | false | Ignore cache, force re-download |
| `--cache-dir` | OS-specific (see Cache) | Custom cache directory |
| `--timeout` | 0 (no limit) | Overall operation timeout in minutes |
| `--layer-timeout` | 30 | Per-layer download timeout in minutes |
| `--retry` | 2 | Number of retries on network errors (0 = no retry) |
| `-q, --quiet` | false | Output only the tar path |
| `-h, --help` | - | Show help |

### `imgp cache` — Cache management

```bash
# Show cache usage
imgp cache info

# Clear all cache
imgp cache clear
```

Default cache locations:

| OS | Path |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache` |
| Linux | `~/.cache/imgp` or `$XDG_CACHE_HOME/imgp` |
| macOS | `~/Library/Caches/imgp` |

Custom directory:

```bash
imgp save hello-world:latest -o hello-world.tar --cache-dir /tmp/my-cache
```

### `imgp gui` — Web GUI

```bash
# Start GUI (default port 9191)
imgp gui

# Custom port
imgp gui -P 9000
```

Features in the browser:

- Enter image name, select platform, click download
- Real-time per-layer progress bars
- Add/remove mirror acceleration entries
- View and clear cache

### `imgp config` — Configuration

```bash
# View current config
imgp config list

# Set mirror map
imgp config set mirror-map "docker.io=docker.m.daocloud.io,gcr.io=gcr.mirrors.daocloud.io"

# Set parallelism
imgp config set parallelism 8

# Add insecure registries (allow HTTP)
imgp config set insecure-registries "192.168.1.100:5000"
```

Config file `imgp.json` is stored next to the binary. Defaults:

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
  },
  "parallelism": 4
}
```

## Mirror Acceleration

`mirror_map` tells imgp which mirror to use for each registry. When pulling `docker.io/library/nginx:latest`, imgp first tries `docker.m.daocloud.io/library/nginx:latest`. If the mirror fails, it falls back to the original `index.docker.io`.

Supported mirrors:

| Registry | Mirror | Provided by |
|---|---|---|
| `docker.io` | `docker.m.daocloud.io` | DaoCloud |
| `gcr.io` | `gcr.mirrors.daocloud.io` | DaoCloud |

Multiple mirrors per registry (separated by `|`):

```bash
imgp config set mirror-map "docker.io=mirror1.example.com|mirror2.example.com"
```

## Demo

```
$ imgp save hello-world:latest -o hello-world.tar

Pulling hello-world:latest (linux/amd64)
Image manifest fetched, downloading layers...
  layers: [1/1] 100% | 2.4 KB / 2.4 KB
    ✓ sha256:4f55086f  100%
  exporting: 100% | 6.5 KB / 6.5 KB
Done: hello-world:latest saved to hello-world.tar
```

With multiple layers:

```
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
```

- `✓` = done
- `◌` = downloading
- `·` = waiting

## Build from source

### Windows

```powershell
# Build current platform only (fast, default)
.\build.ps1

# Build all 6 platforms (for release)
.\build.ps1 -All
```

Output goes to `bin\` directory.

### Linux / macOS

```bash
# Current platform
go build -o imgp .

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o imgp-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## How it works

```
Input: imgp save quay.io/prometheus/node-exporter:v1.11.1 -o out.tar

1. Parse image reference
   → registry = quay.io
   → repository = prometheus/node-exporter
   → tag = v1.11.1

2. Apply mirror map
   → look up mirror_map["quay.io"]
   → try quay.mirrors.daocloud.io first
   → fall back to original quay.io on failure

3. Fetch manifest
   → get manifest list (multi-arch)
   → match target platform (default: linux/amd64)

4. Download layers in parallel
   → each layer gets its own HTTP connection
   → default 4 concurrent downloads
   → real-time progress display

5. Export tar
   → assemble standard Docker tar format
   → write to out.tar

No Docker daemon required
```

## FAQ

### Q: How is this different from `docker pull` + `docker save`?

`docker pull` requires Docker daemon, which on Windows needs WSL2 or Hyper-V. imgp is a single binary with zero dependencies — download and run.

### Q: How to use the exported tar?

```bash
scp hello-world.tar user@remote-server:~/
ssh user@remote-server
docker load -i hello-world.tar
```

### Q: Download was interrupted. What now?

Layers already downloaded are cached. Run the same `imgp save` command again and it will resume. Use `--no-cache` to force a full re-download.

### Q: How does automatic retry work?

On network errors (e.g. `unexpected EOF`, `connection reset`), imgp automatically retries 2 times by default.

```bash
# Retry 5 times
imgp save hello-world:latest -o hello-world.tar --retry 5

# No retry
imgp save hello-world:latest -o hello-world.tar --retry 0

# Persist in config
imgp config set retry 3
```

Errors like 403, 401, 404 are NOT retried.

### Q: Download timed out. What can I do?

Increase the timeout values for large images or slow networks:

```bash
# 60 minutes per layer, 2 hours overall
imgp save large-image:latest -o large.tar --layer-timeout 60 --timeout 120

# Persist in config
imgp config set layer-timeout 60
imgp config set timeout 120
```

Default: 30 minutes per layer, no overall limit (0).

### Q: What platform values are supported?

Format: `os/arch` or `os/arch/variant`

| Platform | Value |
|---|---|
| Linux x86-64 | `linux/amd64` (default) |
| Linux ARM64 | `linux/arm64` |
| Linux ARMv8 | `linux/arm64/v8` |
| Windows x86-64 | `windows/amd64` |
| Windows ARM64 | `windows/arm64` |
| macOS Intel | `darwin/amd64` |
| macOS Apple Silicon | `darwin/arm64` |

### Q: Where is `imgp.json`?

In the same directory as the `imgp` binary. Run `which imgp` (Linux/macOS) or `where.exe imgp` (Windows) to find it.

## Notes

- **Mirror format**: no `https://` prefix needed (e.g., `docker.m.daocloud.io`)
- **Multiple mirrors**: separate with `|` (e.g., `"docker.io=mirror1|mirror2"`)
- **Digest references**: `image@sha256:...` does not trigger mirror acceleration
- **Config changes**: `imgp config set` takes effect immediately

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
