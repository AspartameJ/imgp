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
imgp -v
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

### `imgp` — Global options

| Flag | Description |
|------|-------------|
| `-v, --version` | Print version number |
| `-h, --help` | Show help |

### `imgp save [image...]` — Pull and export

Pull one or more Docker images and save as standard `.tar` files (importable with `docker load`).

```bash
# Single image
imgp save hello-world:latest -o hello-world.tar

# Multiple images (auto-named, -o not allowed)
imgp save nginx:latest redis:latest alpine:latest
```

Full flags:

| Flag | Type | Default | Description |
|---|---|---|---|
| `-o, --output` | string | `image_platform.tar` | Output tar path. Not allowed with multiple images (auto-named) |
| `-p, --platform` | string | `linux/amd64` | Target platform. Format: `os/arch` or `os/arch/variant`, e.g. `linux/arm64`, `linux/arm64/v8`, `windows/amd64` |
| `--username` | string | (empty) | Registry username |
| `--password` | string | (empty) | Registry password. Takes priority over `--password-env`. Note: visible in process listings; use `--password-env` instead |
| `--password-env` | string | `IMG_REGISTRY_PASSWORD` | Env var name holding the password (used when `--password` is not set) |
| `--insecure` | bool | `false` | Allow HTTP connections (skip TLS verify), for internal registries |
| `-P, --parallel` | int | from config, default `4` | Number of parallel layer downloads. Increase for fast networks (e.g. 8), decrease for slow (e.g. 2) |
| `--no-cache` | bool | `false` | Ignore local cache, force re-download all layers |
| `-z, --gzip` | bool | `false` | Gzip-compress the output tar file |
| `--cache-dir` | string | OS default | Custom cache directory. Priority: CLI > config > OS default (see Cache below) |
| `--timeout` | int | `0` (no limit) | Overall operation timeout in minutes, from fetch through export |
| `--layer-timeout` | int | `30` | Per-layer download timeout in minutes. Increase for large images or slow networks; `0` = no limit |
| `--retry` | int | `2` | Number of retries on network errors. Max `30`, `0` = no retry. 4xx errors (401/403/404) are NOT retried |
| `-q, --quiet` | bool | `false` | Quiet mode: output only the tar path. Suitable for scripting |
| `-h, --help` | — | — | Show help for the save command |

### `imgp cache` — Cache management

Manage downloaded layer cache to avoid re-downloading.

#### `imgp cache info`

Show cache usage:

```bash
imgp cache info
```

Sample output:
```
Cache directory: C:\Users\you\AppData\Local\imgp\cache
Cached layers:   12
Total size:      156.3 MB
```

#### `imgp cache clear`

Remove all cached layers:

```bash
imgp cache clear
```

Sample output:
```
Cleared 12 cached layers (156.3 MB)
```

#### `--cache-dir` flag

Both `info` and `clear` support `--cache-dir`:

```bash
imgp cache info --cache-dir /tmp/my-cache
imgp cache clear --cache-dir /tmp/my-cache
```

#### Default cache locations

| OS | Path |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache` (typically `C:\Users\you\AppData\Local\imgp\cache`) |
| Linux | `$XDG_CACHE_HOME/imgp` or `~/.cache/imgp` |
| macOS | `~/Library/Caches/imgp` |

> Priority: `--cache-dir` CLI flag > config `cache_dir` > OS default path

### `imgp config` — Configuration

View and modify persistent configuration stored in `imgp.json`.

#### `imgp config list`

Show all current configuration:

```bash
imgp config list
```

Sample output:
```
Mirror Map: map[docker.io:[docker.m.daocloud.io] ...
Insecure Registries: [192.168.1.100:5000]
Parallelism: 4
Layer Timeout: 30 min
Timeout: 0 min
Retry: 2
Cache Dir: /tmp/my-cache
```

#### `imgp config set <key> <value>`

Supported configuration keys:

| Key | Type | Default | Description |
|---|---|---|---|
| `mirror-map` | string | built-in mirrors | Registry mirror mappings. Format: `reg1=mirror1\|mirror2,reg2=mirror` (commas for groups, pipes for multiple mirrors) |
| `insecure-registries` | string | (empty) | Registries allowed over HTTP (comma-separated) |
| `parallelism` | int | `4` | Number of parallel downloads (minimum 1) |
| `layer-timeout` | int | `30` | Per-layer download timeout in minutes (`0` = no limit) |
| `timeout` | int | `0` | Overall timeout in minutes (`0` = no limit) |
| `retry` | int | `2` | Number of retries (max 30, `0` = no retry) |
| `cache-dir` | string | (empty) | Cache directory path. Set to `""` to fall back to OS default |

Examples:

```bash
imgp config set mirror-map "docker.io=docker.m.daocloud.io,gcr.io=gcr.mirrors.daocloud.io"
imgp config set parallelism 8
imgp config set insecure-registries "192.168.1.100:5000"
imgp config set layer-timeout 60
imgp config set timeout 120
imgp config set retry 3
imgp config set cache-dir "/tmp/my-cache"
imgp config set cache-dir ""      # reset to OS default
```

### Config file `imgp.json`

Stored in the **same directory as the `imgp` binary**. Full structure:

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"],
    "registry.k8s.io": ["m.daocloud.io/registry.k8s.io"]
  },
  "auths": {
    "registry.example.com": {
      "username": "your-username",
      "password_env": "IMG_REGISTRY_PASSWORD"
    }
  },
  "insecure_registries": ["192.168.1.100:5000"],
  "parallelism": 4,
  "layer_timeout": 30,
  "timeout": 0,
  "retry": 2,
  "cache_dir": ""
}
```

Field reference:

| Field | Type | Description |
|---|---|---|
| `mirror_map` | object | Registry → mirror address list mappings |
| `auths` | object | Registry authentication configs. Key is the registry domain, value contains `username`, `password`, `password_env` |
| `insecure_registries` | array | Registries allowed over HTTP |
| `parallelism` | int | Number of parallel downloads |
| `layer_timeout` | int | Per-layer download timeout in minutes (`0` = no limit) |
| `timeout` | int | Overall timeout in minutes (`0` = no limit) |
| `retry` | int | Number of retries |
| `cache_dir` | string | Cache directory path; empty = use OS default |

> The plain-text `password` field inside `auths` is NOT persisted by `imgp config set`. Use `password_env` to reference an environment variable instead.

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

### Q: Can't access Docker Hub. How to configure a mirror?

A mirror is already configured in `mirror_map` by default. To use your own:

```bash
imgp config set mirror-map "docker.io=your-mirror.com"
```

Multiple mirrors (separated by `|`):

```bash
imgp config set mirror-map "docker.io=mirror1.example.com|mirror2.example.com"
```

## Notes

- **Mirror format**: no `https://` prefix needed (e.g., `docker.m.daocloud.io`)
- **Multiple mirrors**: separate with `|` (e.g., `"docker.io=mirror1|mirror2"`)
- **Digest references**: `image@sha256:...` does not trigger mirror acceleration
- **Config changes**: `imgp config set` takes effect immediately

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
