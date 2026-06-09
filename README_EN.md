English | [中文](README.md)

# imgp

Cross-platform Docker image pull and save tool. Supports multi-architecture images, mirror acceleration, parallel downloads, and resume. No Docker daemon required.

## Features

| Feature | Description |
|---|---|
| **Multi-architecture** | Pull images for any platform (`linux/amd64`, `linux/arm64`, `windows/amd64`) |
| **Mirror acceleration** | Built-in mirrors for docker.io, gcr.io |
| **Parallel downloads** | Concurrent layer pulling for faster downloads |
| **Resume support** | Downloaded layers are cached and skipped on retry |
| **Detailed progress** | Per-layer progress bars with real-time updates |
| **Private registries** | Username/password and token authentication |
| **Zero dependencies** | Single binary, pure Go, no Docker installation needed |
| **Cross-platform** | Windows, Linux, and macOS |

## Quick Start

```bash
# Pull and save hello-world
imgp save hello-world:latest -o hello-world.tar

# Specify arm64 architecture
imgp save hello-world:latest --platform linux/arm64 -o hello-world-arm64.tar

# Load into Docker
docker load -i hello-world.tar
```

## Install

### Download binary

Download the pre-built binary from the [Releases page](https://gitcode.com/DonaldTom/imgp/releases) and place it in your `PATH`.

### From source

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

> For users in China who encounter network issues, set Go proxy:
> ```bash
> go env -w GOPROXY=https://goproxy.cn,direct
> ```

## Usage

```bash
imgp save [image] [flags]
```

### Flags

| Flag | Description |
|---|---|
| `-o, --output` | Output tar file path |
| `-p, --platform` | Target platform (default `linux/amd64`, e.g. `linux/arm64`) |
| `--username` | Registry username |
| `--password` | Registry password (use `--password-env` for security) |
| `--password-env` | Env var name for password (default `IMG_REGISTRY_PASSWORD`) |
| `--insecure` | Allow insecure registry connections |
| `-P, --parallel` | Parallel downloads (default: 4) |
| `--no-cache` | Ignore cached layers, force re-download |
| `--cache-dir` | Custom cache directory (see Cache Management for defaults) |
| `-q, --quiet` | Quiet mode, output only the tar path |
| `-h, --help` | Show help |

### Cache Management

```bash
# Show cache usage
imgp cache info

# Clear all cache
imgp cache clear
```

Default cache locations by OS:

| Platform | Path |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache` |
| Linux | `~/.cache/imgp` or `$XDG_CACHE_HOME/imgp` |
| macOS | `~/Library/Caches/imgp` |

Custom directory via `--cache-dir`:

```bash
imgp save -o hello-world.tar hello-world:latest --cache-dir /tmp/my-cache
```

### Configuration

Configuration is stored in `imgp.json` next to the binary. Defaults:

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
  },
  "parallelism": 4
}
```

Edit via CLI:

```bash
# View config
imgp config list

# Set mirror mapping
imgp config set mirror-map "docker.io=docker.m.daocloud.io,gcr.io=gcr.mirrors.daocloud.io"

# Multiple mirrors per registry (separated by |)
imgp config set mirror-map "docker.io=mirror1|mirror2"

# Set parallelism
imgp config set parallelism 8
```

### Authentication

```bash
# From environment variable (recommended)
export IMG_REGISTRY_PASSWORD=your_password
imgp save private/image:latest --username user

# Direct password (not recommended – visible in process listings)
imgp save private/image:latest --username user --password your_password

# Or configure in imgp.json
# See imgp.json.example for details
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

## Build from source

```powershell
# Windows (build all platforms)
.\build.ps1

# Build Windows only
.\build.ps1 -Target windows
```

```bash
# Linux / macOS
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o imgp-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## How it works

1. **Parse** — parse image reference (e.g. `quay.io/prometheus/node-exporter:v1.11.1`)
2. **Mirror** — apply `mirror_map` to replace the registry address
3. **Resolve** — fetch the manifest list and resolve the target platform
4. **Download** — download layers in parallel to a local cache
5. **Export** — assemble cached layers into a standard Docker tar archive

No Docker daemon is required at any step.

## Notes

- **Cache directory** — OS-specific (Windows `%LOCALAPPDATA%`, Linux `~/.cache`, macOS `~/Library/Caches`). Use `imgp cache info` to check usage, `imgp cache clear` to clean up.
- **Mirror format** — do not include `https://` prefix (e.g. `docker.daocloud.io`)
- **Platform format** — `os/arch` or `os/arch/variant` (e.g. `linux/amd64`, `linux/arm64/v8`)
- **Digest references** — `image@sha256:...` format does not trigger auto-mirroring

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
