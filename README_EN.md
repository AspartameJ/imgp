English | [中文](README.md)

# imgp

**imgp** is a Docker image pull and save tool for Windows. Single binary, zero dependencies, no Docker daemon required.

```
imgp save hello-world:latest -o hello-world.tar
docker load -i hello-world.tar    # import on a machine with Docker
```

> Source code is cross-platform (compiles on Linux/macOS), but only Windows/amd64 binaries and CI are provided.

---

## Install

### Download binary

Download `imgp-windows-amd64.exe` from [Releases](https://gitcode.com/DonaldTom/imgp/releases) and place it in your `PATH`.

### Go install

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

For users in China:

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### Verify

```bash
imgp -v
```

---

## Quick Start

```bash
# Pull and save
imgp save hello-world:latest

# Specify platform
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# Private registry
export IMG_REGISTRY_PASSWORD=your_password
imgp save private.registry.com/myapp:latest --username user

# Multiple images (auto-named)
imgp save nginx:latest redis:latest alpine:latest

# Gzip compression
imgp save nginx:latest -z -o nginx.tar.gz
```

---

## Command Reference

### `imgp save [image...]` — Pull and export

| Flag | Type | Default | Description |
|---|---|---|---|
| `-o, --output` | string | `image_platform.tar` | Output path. Not allowed for multiple images |
| `-p, --platform` | string | `linux/amd64` | Target platform, e.g. `linux/arm64`, `windows/amd64` |
| `--username` | string | — | Registry username |
| `--password` | string | — | Password (visible in process listing; use `--password-env`) |
| `--password-env` | string | — | Env var name holding the password |
| `--insecure` | bool | `false` | Skip TLS verify (internal HTTP registries) |
| `-P, --parallel` | int | `4` | Parallel layer downloads |
| `--no-cache` | bool | `false` | Ignore cache, force re-download |
| `-z, --gzip` | bool | `false` | Gzip-compress output |
| `--cache-dir` | string | OS default | Custom cache directory |
| `--timeout` | int | `0`(unlimited) | Overall timeout (minutes) |
| `--layer-timeout` | int | `30` | Per-layer timeout (minutes) |
| `--retry` | int | `2` | Retry count (`0` = no retry, max `30`) |
| `-q, --quiet` | bool | `false` | Quiet mode, output path only |

### `imgp cache` — Cache management

```bash
imgp cache info                # show cache usage
imgp cache clear               # remove all cached layers
imgp cache info --cache-dir /tmp/my-cache
```

| OS | Default cache path |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache` |
| Linux | `$XDG_CACHE_HOME/imgp` or `~/.cache/imgp` |
| macOS | `~/Library/Caches/imgp` |

> Priority: `--cache-dir` CLI > config `cache_dir` > OS default

### `imgp config` — Configuration

```bash
imgp config list                                          # view config
imgp config set mirror-map "docker.io=my-mirror.com"      # set mirror
imgp config set parallelism 8                             # concurrency
imgp config set retry 3                                   # retries
imgp config set cache-dir "/tmp/my-cache"                 # cache dir
imgp config set insecure-registries "192.168.1.100:5000"  # HTTP registries
imgp config set layer-timeout 60                          # layer timeout
imgp config set timeout 120                               # overall timeout
```

Config file `imgp.json` lives alongside the binary:

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
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

> The `password` field is NOT persisted. Use `password_env` to reference an environment variable.

---

## Mirror Acceleration

Default mirrors for fast access in China:

| Registry | Mirror | Provider |
|---|---|---|
| `docker.io` | `docker.m.daocloud.io` | DaoCloud |
| `gcr.io` | `gcr.mirrors.daocloud.io` | DaoCloud |
| `registry.k8s.io` | `m.daocloud.io/registry.k8s.io` | DaoCloud |
| `quay.io` | `quay.nju.edu.cn` | Nanjing University |

How it works: pulling `docker.io/library/nginx:latest` → tries mirror first → falls back to original on failure.

```bash
# Custom mirror
imgp config set mirror-map "docker.io=my-mirror.com"

# Multiple mirrors (separated by |, tried in order)
imgp config set mirror-map "docker.io=mirror1.example.com|mirror2.example.com"
```

---

## Demo

```text
$ imgp save hello-world:latest -o hello-world.tar

Pulling hello-world:latest (linux/amd64)
Image manifest fetched, downloading layers...
  layers: [1/1] 100% | 2.4 KB / 2.4 KB
    ✓ sha256:4f55086f  100%
  exporting: 100% | 6.5 KB / 6.5 KB
Done: hello-world:latest saved to hello-world.tar
```

Multiple layers:

```text
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
```

- `✓` = done | `◌` = downloading | `·` = waiting

---

## How It Works

```text
Input: imgp save quay.io/prometheus/node-exporter:v1.11.1 -o out.tar

1. Parse image reference → registry / repository / tag
2. Apply mirror map → check mirror_map, try mirror first
3. Fetch manifest → match target platform
4. Download layers in parallel → 4 concurrent, real-time progress
5. Export tar → standard Docker tar format

No Docker daemon required
```

---

## FAQ

### How is this different from `docker pull` + `docker save`?

`docker pull` needs the Docker daemon (requires WSL2/Hyper-V on Windows). imgp is a single binary with zero dependencies.

### Download interrupted?

Cached layers resume automatically. Run the same command again. Use `--no-cache` to force re-download.

```bash
# Auto retry (default 2 times)
imgp save hello-world:latest --retry 5

# Timeout control
imgp save large-image:latest --layer-timeout 60 --timeout 120
```

4xx errors (401/403/404) are NOT retried.

### What `--platform` values are supported?

Format: `os/arch` or `os/arch/variant`:

| Platform | Value |
|---|---|
| Linux x86-64 | `linux/amd64` (default) |
| Linux ARM64 | `linux/arm64` |
| Linux ARMv8 | `linux/arm64/v8` |
| Windows x86-64 | `windows/amd64` |
| Windows ARM64 | `windows/arm64` |
| macOS Intel | `darwin/amd64` |
| macOS Apple Silicon | `darwin/arm64` |

### Can't access Docker Hub?

Default mirrors are pre-configured. To use your own:

```bash
imgp config set mirror-map "docker.io=your-mirror.com"
```

---

## License

GNU General Public License v3.0. See [LICENSE](LICENSE).
