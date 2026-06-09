# imgp

Cross-platform Docker image pull and save tool. Pulls multi-architecture images from registries with mirror acceleration support and saves them as standard `docker load`-compatible tar archives — no Docker daemon required.

## Features

- **Multi-architecture** — pull images for any platform (`linux/amd64`, `linux/arm64`, `windows/amd64`, etc.)
- **Mirror acceleration** — automatic mirror resolution for Docker Hub, quay.io, gcr.io
- **Parallel downloads** — concurrent layer pulling with configurable concurrency
- **Resume support** — partially downloaded layers are cached for resumption
- **Detailed progress** — multi-line progress display with per-layer bars
- **Private registries** — username/password and token authentication
- **No dependencies** — single binary, pure Go, zero system deps (no Docker needed)
- **Windows-friendly** — fully compatible with Windows, Linux, and macOS

## Install

### From source

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

### Download binary

Download the pre-built binary from the [Releases](https://gitcode.com/DonaldTom/imgp/releases) page.

## Quick Start

```bash
# Save nginx for your current platform
imgp save nginx:latest -o nginx.tar

# Save for a specific architecture
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# Load into Docker
docker load -i nginx.tar
```

## Usage

```bash
imgp save [image] [flags]
```

### Flags

| Flag | Description |
|---|---|
| `-o, --output` | Output tar file path |
| `-p, --platform` | Target platform (e.g. `linux/amd64`, `linux/arm64`) |
| `--username` | Registry username |
| `--password` | Registry password |
| `--password-env` | Environment variable for password (default `IMG_REGISTRY_PASSWORD`) |
| `--insecure` | Allow insecure registry connections |
| `-P, --parallel` | Number of parallel downloads (default: from config, or 4) |
| `-q, --quiet` | Quiet mode, output only the tar path |
| `-h, --help` | Show help |

### Configuration

Configuration is stored in `imgp.json` next to the binary. Default values:

```json
{
  "mirror_map": {
    "docker.io": ["docker.daocloud.io"],
    "quay.io": ["quay.mirrors.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
  },
  "parallelism": 4
}
```

Edit via CLI:

```bash
imgp config list
imgp config set mirror-map "docker.io=docker.daocloud.io,quay.io=quay.mirrors.daocloud.io"
imgp config set parallelism 8
```

### Authentication

```bash
# Password as argument
imgp save myregistry.com/private/app:latest --username user --password pass123

# Password from environment variable
export IMG_REGISTRY_PASSWORD=pass123
imgp save myregistry.com/private/app:latest --username user

# Or configure in imgp.json
# See imgp.json.example for details
```

## Build from source

```powershell
# Windows
.\build.ps1
```

```bash
# Linux / macOS
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## How it works

1. Parse image reference (e.g. `quay.io/prometheus/node-exporter:v1.11.1`)
2. Apply mirror map (e.g. `quay.io` → `quay.mirrors.daocloud.io`)
3. Fetch manifest list and resolve the target platform
4. Download layers in parallel with progress reporting
5. Assemble and write a standard Docker tar archive

No Docker daemon is required at any step.

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
