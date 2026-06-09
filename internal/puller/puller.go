package puller

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
}

func NewPuller(cacheDir string) *Puller {
	return &Puller{
		cacheDir:     cacheDir,
		layerTimeout: 30 * time.Minute,
	}
}

func (p *Puller) WithNoCache(v bool) *Puller {
	p.noCache = v
	return p
}

func (p *Puller) WithLayerTimeout(d time.Duration) *Puller {
	if d > 0 {
		p.layerTimeout = d
	}
	return p
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
						sendEvent(ctx, ch, PullEvent{
							Index: t.Index, Digest: t.DigestHex,
							Bytes: t.Size, Total: t.Size, Status: "cached",
						})
						return
					}
				}

				os.Remove(cacheFile)

				if !sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "downloading",
				}) {
					return
				}

				layerCtx, cancel := context.WithTimeout(ctx, p.layerTimeout)
				defer cancel()

				rc, err := t.OpenLayer(layerCtx)
				if err != nil {
					sendEvent(ctx, ch, PullEvent{
						Index: t.Index, Digest: t.DigestHex,
						Err: fmt.Errorf("open layer: %w", err), Status: "error",
					})
					return
				}
				defer rc.Close()

				f, err := os.Create(cacheFile)
				if err != nil {
					sendEvent(ctx, ch, PullEvent{
						Index: t.Index, Digest: t.DigestHex,
						Err: fmt.Errorf("create cache: %w", err), Status: "error",
					})
					return
				}
				defer f.Close()

				buf := make([]byte, 64*1024)
				var written int64
				lastReport := time.Now()

				for {
					n, readErr := rc.Read(buf)
					if n > 0 {
						if _, werr := f.Write(buf[:n]); werr != nil {
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
						os.Remove(cacheFile)
						sendEvent(ctx, ch, PullEvent{
							Index: t.Index, Digest: t.DigestHex,
							Err: fmt.Errorf("read layer: %w", readErr), Status: "error",
						})
						return
					}
				}

				sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex,
					Bytes: t.Size, Total: t.Size, Status: "done",
				})
			}(task)
		}

		wg.Wait()
	}()

	return ch, nil
}
