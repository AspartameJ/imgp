package puller

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gitcode.com/DonaldTom/imgp/internal/util"
)

// PullEvent represents a progress or error event during layer download.
type PullEvent struct {
	Index  int
	Digest string
	Bytes  int64
	Total  int64
	Status string
	Err    error
}

// LayerTask describes a single layer to download.
type LayerTask struct {
	Index     int
	DigestHex string
	Size      int64
	OpenLayer func(ctx context.Context) (io.ReadCloser, error)
}

// Puller manages concurrent layer downloads with caching and retry.
type Puller struct {
	cacheDir     string
	noCache      bool
	layerTimeout time.Duration
	maxRetries   int
}

// NewPuller creates a Puller with the given cache directory.
func NewPuller(cacheDir string) *Puller {
	return &Puller{
		cacheDir:     cacheDir,
		layerTimeout: 30 * time.Minute,
		maxRetries:   2,
	}
}

// WithNoCache sets whether to ignore cached layers.
func (p *Puller) WithNoCache(v bool) *Puller {
	p.noCache = v
	return p
}

// WithLayerTimeout sets the per-layer download timeout.
func (p *Puller) WithLayerTimeout(d time.Duration) *Puller {
	p.layerTimeout = d
	return p
}

// WithRetry sets the max retry count for network errors.
func (p *Puller) WithRetry(n int) *Puller {
	if n >= 0 {
		p.maxRetries = n
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

// checkCache verifies and uses a cached layer; returns true if cache was used.
func (p *Puller) checkCache(ctx context.Context, ch chan<- PullEvent, t LayerTask, cacheFile string) bool {
	if p.noCache {
		return false
	}
	fi, err := os.Stat(cacheFile)
	if err != nil || fi.Size() != t.Size {
		return false
	}
	f, e := os.Open(cacheFile)
	if e != nil {
		return false
	}
	defer f.Close()
	var magic [2]byte
	if _, err := f.Read(magic[:]); err != nil {
		return false
	}
	if magic[0] != 0x1f || magic[1] != 0x8b {
		return false
	}
	if !sendEvent(ctx, ch, PullEvent{
		Index: t.Index, Digest: t.DigestHex,
		Bytes: t.Size, Total: t.Size, Status: "cached",
	}) {
		return false
	}
	return true
}

// backoffWait sleeps with exponential backoff; returns false if context was cancelled.
func (p *Puller) backoffWait(ctx context.Context, ch chan<- PullEvent, t LayerTask, attempt int) bool {
	if err := util.Backoff(ctx, attempt); err != nil {
		sendEvent(ctx, ch, PullEvent{
			Index: t.Index, Digest: t.DigestHex,
			Err: ctx.Err(), Status: "error",
		})
		return false
	}
	return true
}

// downloadAttempt performs a single download attempt; returns true on success.
// On failure, lastErr holds the error and should be checked by the caller.
func (p *Puller) downloadAttempt(ctx context.Context, ch chan<- PullEvent, t LayerTask, cacheFile string) (ok bool, lastErr error) {
	var layerCtx context.Context
	var cancel context.CancelFunc
	if p.layerTimeout > 0 {
		layerCtx, cancel = context.WithTimeout(ctx, p.layerTimeout)
	} else {
		layerCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	rc, openErr := t.OpenLayer(layerCtx)
	if openErr != nil {
		return false, openErr
	}
	defer rc.Close()

	f, createErr := os.Create(cacheFile)
	if createErr != nil {
		return false, fmt.Errorf("create cache: %w", createErr)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && lastErr == nil {
			lastErr = fmt.Errorf("close cache file: %w", cerr)
		}
	}()

	buf := make([]byte, 64*1024)
	var written int64
	lastReport := time.Now()

	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return false, werr
			}
			written += int64(n)
			if time.Since(lastReport) > 200*time.Millisecond {
				if !sendEvent(ctx, ch, PullEvent{
					Index: t.Index, Digest: t.DigestHex,
					Bytes: written, Total: t.Size, Status: "downloading",
				}) {
					return false, ctx.Err()
				}
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return false, readErr
		}
	}

	if written != t.Size {
		return false, fmt.Errorf("incomplete download: got %d, expected %d", written, t.Size)
	}
	return true, nil
}

// processTask handles a single layer task: check cache, download with retry, send result event.
func (p *Puller) processTask(ctx context.Context, ch chan<- PullEvent, t LayerTask) {
	if !sendEvent(ctx, ch, PullEvent{
		Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "starting",
	}) {
		return
	}

	cacheFile := filepath.Join(p.cacheDir, t.DigestHex+".gz")

	if p.checkCache(ctx, ch, t, cacheFile) {
		return
	}
	if ctx.Err() != nil {
		return
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
	var attempt int
	for attempt = 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			if !util.IsRetryable(lastErr) {
				break
			}
			if !p.backoffWait(ctx, ch, t, attempt) {
				return
			}
			if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "pull: remove before retry: %v\n", err)
			}
			if !sendEvent(ctx, ch, PullEvent{
				Index: t.Index, Digest: t.DigestHex, Total: t.Size, Status: "downloading",
			}) {
				return
			}
		}

		ok, err := p.downloadAttempt(ctx, ch, t, cacheFile)
		if ok {
			sendEvent(ctx, ch, PullEvent{
				Index: t.Index, Digest: t.DigestHex,
				Bytes: t.Size, Total: t.Size, Status: "done",
			})
			return
		}
		lastErr = err
		if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "pull: remove after failed attempt: %v\n", err)
		}
	}

	attemptLabel := "attempts"
	if attempt == 1 {
		attemptLabel = "attempt"
	}
	sendEvent(ctx, ch, PullEvent{
		Index: t.Index, Digest: t.DigestHex,
		Err:    fmt.Errorf("download failed after %d %s: %w", attempt, attemptLabel, lastErr),
		Status: "error",
	})
}

// Pull downloads layers concurrently and sends progress events on the returned channel.
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
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "PANIC in puller worker: %v\n", r)
						sendEvent(ctx, ch, PullEvent{
							Index: t.Index, Digest: t.DigestHex,
							Err: fmt.Errorf("panic: %v", r), Status: "error",
						})
					}
				}()
				p.processTask(ctx, ch, t)
			}(task)
		}

		wg.Wait()
	}()

	return ch, nil
}
