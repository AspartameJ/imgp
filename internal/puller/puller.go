package puller

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PullEvent struct {
	Index  int
	Digest string
	Bytes  int64
	Total  int64
	Status string
	Err    error
}

type LayerTask struct {
	Index     int
	DigestHex string
	Size      int64
	OpenLayer func(ctx context.Context) (io.ReadCloser, error)
}

type Puller struct {
	cacheDir     string
	noCache      bool
	layerTimeout time.Duration
	maxRetries   int
}

func NewPuller(cacheDir string) *Puller {
	return &Puller{
		cacheDir:     cacheDir,
		layerTimeout: 30 * time.Minute,
		maxRetries:   2,
	}
}

func (p *Puller) WithNoCache(v bool) *Puller {
	p.noCache = v
	return p
}

func (p *Puller) WithLayerTimeout(d time.Duration) *Puller {
	p.layerTimeout = d
	return p
}

func (p *Puller) WithRetry(n int) *Puller {
	if n >= 0 {
		p.maxRetries = n
	}
	return p
}

func isRetryable(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "403") || strings.Contains(msg, "401") ||
		strings.Contains(msg, "404") || strings.Contains(msg, "405") {
		return false
	}
	retryable := []string{"unexpected EOF", "connection reset", "connection refused",
		"timeout", "TLS handshake", "no such host", "dial tcp", "broken pipe", "i/o timeout"}
	for _, s := range retryable {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func sendEvent[T any](ctx context.Context, ch chan<- T, evt T) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- evt:
		return true
	}
}

func (p *Puller) Pull(
	ctx context.Context,
	tasks []LayerTask,
	parallel int,
) (<-chan PullEvent, error) {
	if parallel < 1 {
		parallel = 1
	}

	if err := os.MkdirAll(p.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	ch := make(chan PullEvent, parallel*2)

	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		sem := make(chan struct{}, parallel)

		for _, task := range tasks {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(t LayerTask) {
				defer wg.Done()
				defer func() { <-sem }()

				if !sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "starting",
				}) {
					return
				}

				cacheFile := filepath.Join(p.cacheDir, t.DigestHex+".gz")

	if !p.noCache {
				if fi, err := os.Stat(cacheFile); err == nil && fi.Size() == t.Size {
					if f, e := os.Open(cacheFile); e == nil {
						var magic [2]byte
						if _, err := f.Read(magic[:]); err != nil {
							f.Close()
							sendEvent(ctx, ch, PullEvent{
								Index: t.Index, Digest: t.DigestHex,
								Err: fmt.Errorf("read cache: %w", err),
							})
							return
						}
						f.Close()
						if magic[0] == 0x1f && magic[1] == 0x8b {
							if !sendEvent(ctx, ch, PullEvent{
								Index: t.Index, Digest: t.DigestHex,
								Bytes: t.Size, Total: t.Size, Status: "cached",
							}) {
								return
							}
							return
						}
					}
				}
				}

			if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
				sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex,
					Err: fmt.Errorf("remove cache: %w", err),
				})
				return
			}

			if !sendEvent(ctx, ch, PullEvent{
				Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "downloading",
			}) {
					return
				}

				var lastErr error
				for attempt := 0; attempt <= p.maxRetries; attempt++ {
					if attempt > 0 {
						if !isRetryable(lastErr) {
							break
						}
						backoff := time.Duration(1<<uint(attempt-1)) * time.Second
						select {
						case <-ctx.Done():
							sendEvent(context.Background(), ch, PullEvent{
								Index: t.Index, Digest: t.DigestHex,
								Err: ctx.Err(), Status: "error",
							})
							return
						case <-time.After(backoff):
						}
						if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
							sendEvent(ctx, ch, PullEvent{
								Index: t.Index, Digest: t.DigestHex,
								Err: fmt.Errorf("remove cache: %w", err),
							})
							return
						}
						if !sendEvent(ctx, ch, PullEvent{
							Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "downloading",
						}) {
							return
						}
					}

					var layerCtx context.Context
				var cancel context.CancelFunc
				if p.layerTimeout > 0 {
					layerCtx, cancel = context.WithTimeout(ctx, p.layerTimeout)
				} else {
					layerCtx, cancel = context.WithCancel(ctx)
				}
					rc, openErr := t.OpenLayer(layerCtx)
					if openErr != nil {
						cancel()
						lastErr = openErr
						continue
					}

					f, createErr := os.Create(cacheFile)
					if createErr != nil {
						rc.Close()
						cancel()
						sendEvent(ctx, ch, PullEvent{
							Index: t.Index, Digest: t.DigestHex,
							Err: fmt.Errorf("create cache: %w", createErr), Status: "error",
						})
						return
					}

					buf := make([]byte, 64*1024)
					var written int64
					lastReport := time.Now()
					readFailed := false

					for {
						n, readErr := rc.Read(buf)
						if n > 0 {
							if _, werr := f.Write(buf[:n]); werr != nil {
								rc.Close()
								f.Close()
								cancel()
								os.Remove(cacheFile)
								sendEvent(ctx, ch, PullEvent{
									Index: t.Index, Digest: t.DigestHex,
									Err: fmt.Errorf("write cache: %w", werr), Status: "error",
								})
								return
							}
							written += int64(n)
							if time.Since(lastReport) > 200*time.Millisecond {
								sendEvent(ctx, ch, PullEvent{
									Index: t.Index, Digest: t.DigestHex,
									Bytes: written, Total: t.Size, Status: "downloading",
								})
								lastReport = time.Now()
							}
						}
						if readErr == io.EOF {
							break
						}
						if readErr != nil {
							lastErr = readErr
							readFailed = true
							break
						}
					}

					rc.Close()
					f.Close()
					cancel()

					if readFailed {
						os.Remove(cacheFile)
						continue
					}

					sendEvent(ctx, ch, PullEvent{
						Index: t.Index, Digest: t.DigestHex,
						Bytes: t.Size, Total: t.Size, Status: "done",
					})
					return
				}

				sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex,
					Err: fmt.Errorf("download failed after %d attempts: %w", p.maxRetries+1, lastErr),
					Status: "error",
				})
			}(task)
		}

		wg.Wait()
	}()

	return ch, nil
}
