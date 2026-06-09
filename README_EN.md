English | [中文](README.md)

# imgp

Cross-platform Docker image pull and save tool. Supports multi-architecture images, mirror acceleration, parallel downloads, and resume. No Docker daemon required.

## Features

| Feature | Description |
|---|---|
| **Multi-architecture** | Pull images for any platform (`linux/amd64`, `linux/arm64`, `windows/amd64`) |
| **Mirror acceleration** | Built-in mirrors for docker.io, quay.io, gcr.io |
| **Parallel downloads** | Concurrent layer pulling for faster downloads |
| **Resume support** | Downloaded layers are cached and skipped on retry |
| **Detailed progress** | Per-layer progress bars with real-time updates |
| **Private registries** | Username/password and token authentication |
| **Zero dependencies** | Single binary, pure Go, no Docker installation needed |
| **Cross-platform** | Windows, Linux, and macOS |

## Quick Start

```bash
# Pull and save the latest nginx
imgp save nginx:latest -o nginx.tar

# Specify arm64 architecture
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# Load into Docker
docker load -i nginx.tar
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
| `-q, --quiet` | Quiet mode, output only the tar path |
| `-h, --help` | Show help |

### Configuration

Configuration is stored in `imgp.json` next to the binary. Defaults:

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "quay.io": ["quay.mirrors.daocloud.io"],
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
imgp config set mirror-map "docker.io=docker.m.daocloud.io,quay.io=quay.mirrors.daocloud.io"

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
$ imgp save nginx:latest -o nginx.tar --platform linux/arm64

Pulling nginx:latest (linux/arm64)
Image manifest fetched, downloading layers...
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
  exporting: 100% | 11.8 MB / 11.8 MB
Done: nginx:latest (linux/arm64) saved to nginx_linux-arm64.tar
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

- **Cache directory** — `.imgp-cache/` is created alongside the binary. Delete it to free up space.
- **Mirror format** — do not include `https://` prefix (e.g. `docker.daocloud.io`)
- **Platform format** — `os/arch` or `os/arch/variant` (e.g. `linux/amd64`, `linux/arm64/v8`)
- **Digest references** — `image@sha256:...` format does not trigger auto-mirroring

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
