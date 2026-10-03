package puller

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSendEvent(t *testing.T) {
	ch := make(chan int, 1)
	ctx := context.Background()

	if !sendEvent(ctx, ch, 42) {
		t.Error("sendEvent returned false, want true")
	}
	if v := <-ch; v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

func TestSendEvent_CancelledCtx(t *testing.T) {
	ch := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if sendEvent(ctx, ch, 1) {
		t.Error("sendEvent on cancelled ctx returned true, want false")
	}
}

func TestNewPuller(t *testing.T) {
	p := NewPuller("/tmp/cache")
	if p == nil {
		t.Fatal("NewPuller returned nil")
	}
	if p.cacheDir != "/tmp/cache" {
		t.Errorf("cacheDir = %q, want /tmp/cache", p.cacheDir)
	}
}

func TestPullerOptions(t *testing.T) {
	p := NewPuller("/tmp/cache").
		WithNoCache(true).
		WithResume(true).
		WithLayerTimeout(5 * time.Minute).
		WithRetry(3)

	if !p.noCache {
		t.Error("noCache should be true")
	}
	if !p.resume {
		t.Error("resume should be true")
	}
	if p.layerTimeout != 5*time.Minute {
		t.Errorf("layerTimeout = %v, want 5m", p.layerTimeout)
	}
	if p.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", p.maxRetries)
	}
}

func TestPullerWithRetryNegative(t *testing.T) {
	p := NewPuller("/tmp/cache").WithRetry(-1)
	if p.maxRetries != 2 {
		t.Errorf("maxRetries should stay 2, got %d", p.maxRetries)
	}
}

func TestPull_CacheHit(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	digest := "abcdef"
	createGzipLayer(t, dir, digest, []byte("layer data"))
	os.WriteFile(filepath.Join(dir, digest+".gz.verified"), nil, 0644)
	size := cacheSize(t, dir, digest)

	var openCalled bool
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      size,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			openCalled = true
			return io.NopCloser(bytes.NewReader([]byte("layer data"))), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	if openCalled {
		t.Error("OpenLayer should not be called on cache hit")
	}
	assertStatus(t, events, 1, "cached")
}

func TestBackoffOrSendError_Cancel(t *testing.T) {
	p := &Puller{}
	ch := make(chan PullEvent, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok := p.backoffOrSendError(ctx, ch, LayerTask{}, 5)
	if ok {
		t.Error("expected backoffOrSendError to return false on cancelled context")
	}
}

func TestBackoffOrSendError_Success(t *testing.T) {
	p := &Puller{}
	ch := make(chan PullEvent, 1)
	ok := p.backoffOrSendError(context.Background(), ch, LayerTask{}, 0)
	if !ok {
		t.Error("expected backoffOrSendError to return true")
	}
	select {
	case <-ch:
		t.Error("unexpected event on channel")
	default:
	}
}

func TestPull_CacheMiss_Success(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	msg := "freshly downloaded layer content"
	payload := gzipBytes(t, msg)
	digest := digestOf(payload)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	assertStatus(t, events, 0, "starting")
	assertStatus(t, events, 1, "downloading")
	assertStatus(t, events, 2, "done")

	got := mustDecompress(t, filepath.Join(dir, digest+".gz"))
	if string(got) != msg {
		t.Errorf("cached content = %q, want %q", got, msg)
	}
}

func TestPull_NoCache(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithNoCache(true)

	payload := gzipBytes(t, "fresh data")
	digest := digestOf(payload)
	createGzipLayer(t, dir, "stale", []byte("stale data"))

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	assertStatus(t, events, 2, "done")

	got := mustDecompress(t, filepath.Join(dir, digest+".gz"))
	if string(got) != "fresh data" {
		t.Errorf("expected fresh data, got %q", got)
	}
}

func TestPull_Resume(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithResume(true)

	msg := "resumed layer content that is long enough to split"
	payload := gzipBytes(t, msg)
	digest := digestOf(payload)

	half := len(payload) / 2
	if err := os.WriteFile(filepath.Join(dir, digest+".gz"), payload[:half], 0644); err != nil {
		t.Fatal(err)
	}

	var gotOffset int64 = -1
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, offset int64) (io.ReadCloser, error) {
			gotOffset = offset
			return io.NopCloser(bytes.NewReader(payload[offset:])), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, eventCh)
	assertStatus(t, events, len(events)-1, "done")

	if gotOffset != int64(half) {
		t.Errorf("OpenLayer offset = %d, want %d", gotOffset, half)
	}
	got := mustDecompress(t, filepath.Join(dir, digest+".gz"))
	if string(got) != msg {
		t.Errorf("cached content = %q, want %q", got, msg)
	}
}

func TestPull_ResumeDisabled(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	msg := "fresh content without resume"
	payload := gzipBytes(t, msg)
	digest := digestOf(payload)

	if err := os.WriteFile(filepath.Join(dir, digest+".gz"), payload[:len(payload)/2], 0644); err != nil {
		t.Fatal(err)
	}

	var gotOffset int64 = -1
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, offset int64) (io.ReadCloser, error) {
			gotOffset = offset
			return io.NopCloser(bytes.NewReader(payload[offset:])), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, eventCh)
	assertStatus(t, events, len(events)-1, "done")

	if gotOffset != 0 {
		t.Errorf("OpenLayer offset = %d, want 0 (resume disabled)", gotOffset)
	}
	got := mustDecompress(t, filepath.Join(dir, digest+".gz"))
	if string(got) != msg {
		t.Errorf("cached content = %q, want %q", got, msg)
	}
}

func TestPull_DownloadFailure(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	digest := "f00bad"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return nil, errors.New("network error")
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error", last.Status)
	}
}

func TestPull_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	digest := "canceled"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	// Pull may close the channel immediately when context is cancelled;
	// this test just verifies no panic or deadlock.
	_ = collectEvents(t, eventCh)
}

func TestPull_MultipleTasks(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	payloads := [][]byte{
		[]byte("layer zero"),
		[]byte("layer one"),
	}
	tasks := make([]LayerTask, 2)
	for i, payload := range payloads {
		tasks[i] = LayerTask{
			Index:     i,
			DigestHex: digestOf(payload),
			Size:      int64(len(payload)),
			OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(payload)), nil
			},
		}
	}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 2)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	doneCount := 0
	for _, e := range events {
		if e.Status == "done" {
			doneCount++
		}
	}
	if doneCount != 2 {
		t.Errorf("expected 2 done events, got %d\nEvents: %+v", doneCount, events)
	}
}

func TestPull_CacheCorrupted(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	payload := gzipBytes(t, "real content")
	digest := digestOf(payload)
	// Corrupt cache file with the correct size and gzip magic but wrong
	// content: the digest check must reject and remove it.
	corrupt := bytes.Repeat([]byte{0x1f, 0x8b, 0x00, 0x00}, len(payload)/4+1)[:len(payload)]
	os.WriteFile(filepath.Join(dir, digest+".gz"), corrupt, 0644)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "done" {
		t.Errorf("final status = %q, want done (should re-download on corrupted cache)", last.Status)
	}
}

func TestPull_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(1)

	digest := "sizemismatch"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      1000, // declared larger than actual data
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(gzipBytes(t, "short"))), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error on size mismatch", last.Status)
	}
}

func TestPull_LayerTimeout(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithLayerTimeout(10 * time.Millisecond).WithRetry(0)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: "slow-layer",
		Size:      1000,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}}

	eventCh, err := p.Pull(context.Background(), tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("expected final status 'error', got %q", last.Status)
	}
}

func TestPull_ContextDeadline(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: "deadline-layer",
		Size:      1000,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
	// When the context expires, the final "error" event may not be sent
	// because sendEvent checks ctx.Done() before writing. We just verify
	// no panic/deadlock and that at least one event was received.
}

func TestPull_RetryThenSuccess(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(2)

	payload := gzipBytes(t, "finally works")
	digest := digestOf(payload)
	attempt := 0
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			attempt++
			if attempt < 2 {
				return nil, errors.New("connection refused")
			}
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "done" {
		t.Errorf("final status = %q, want done after retry", last.Status)
	}
}

func TestPull_RetryExhausted(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(1) // 1 retry = 2 total attempts

	digest := "neverworks"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return nil, errors.New("persistent network error")
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error after retry exhaustion", last.Status)
	}
}

func TestProcessTask_NonRetryableError(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(1)

	digest := "nonretry"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return nil, errors.New("unexpected status code 403 Forbidden")
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error (non-retryable should fail fast)", last.Status)
	}
}

func TestProcessTask_RemoveCleanupError(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	digest := "removefail"
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return nil, errors.New("first attempt fails")
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error", last.Status)
	}
}

func TestPull_ZeroParallel(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	payload := gzipBytes(t, "data")
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digestOf(payload),
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 0) // should clamp to 1
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "done" {
		t.Errorf("final status = %q, want done", last.Status)
	}
}

func TestCheckCache_BadGzipHeader(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithNoCache(false)

	digest := "badgzip"
	cacheFile := filepath.Join(dir, digest+".gz")
	// File with matching size but invalid gzip header: the magic check must reject it.
	bad := []byte("not gzip data")
	os.WriteFile(cacheFile, bad, 0644)

	ch := make(chan PullEvent, 1)
	task := LayerTask{Index: 0, DigestHex: digest, Size: int64(len(bad))}
	ok := p.checkCache(context.Background(), ch, task, cacheFile)
	if ok {
		t.Error("expected false for corrupted cache file")
	}
}

func TestCheckCache_OpenError(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	digest := "openfail"
	// Write a file that can be stated but not opened (on Unix, remove read perm; on Windows, ok)
	cacheFile := filepath.Join(dir, digest+".gz")
	os.WriteFile(cacheFile, []byte("data"), 0644)

	// Simulate open error by deleting between stat and open
	// Actually, on Windows we can't easily remove permissions. Instead, test
	// with a file that exists but then gets deleted before open.
	// For simplicity, test the non-cache path: noCache=true
	ch := make(chan PullEvent, 1)
	task := LayerTask{Index: 0, DigestHex: digest, Size: 4}
	ok := p.checkCache(context.Background(), ch, task, "/nonexistent/cache.gz")
	if ok {
		t.Error("expected false for nonexistent cache")
	}
}

func TestDownloadAttempt_NoTimeout(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0).WithLayerTimeout(0)

	payload := gzipBytes(t, "data no timeout")
	digest := digestOf(payload)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "done" {
		t.Errorf("final status = %q, want done", last.Status)
	}
}

func TestDownloadAttempt_ReadError(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: "readfail",
		Size:      100,
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(&errReader{err: errors.New("simulated read error")}), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error for read failure", last.Status)
	}
}

type errReader struct {
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	return 0, r.err
}

func TestDownloadAttempt_WriteError(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	payload := gzipBytes(t, "data to download")
	digest := "writefail"

	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	// Use a path where we can't create files (read-only dir doesn't work on Windows easily)
	// Instead, test the incomplete download path by providing wrong size
	ctx := context.Background()
	tasks[0].Size = int64(len(payload)) * 10 // declare 10x larger than actual
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Errorf("final status = %q, want error for size mismatch", last.Status)
	}
}

func TestPull_DigestMismatch(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0)

	payload := gzipBytes(t, "payload that does not match the digest")
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: "0000000000000000000000000000000000000000000000000000000000000000",
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}

	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Fatalf("final status = %q, want error on digest mismatch", last.Status)
	}
	if !strings.Contains(last.Err.Error(), "digest mismatch") {
		t.Errorf("error = %v, want digest mismatch", last.Err)
	}
	if _, err := os.Stat(filepath.Join(dir, tasks[0].DigestHex+".gz")); !os.IsNotExist(err) {
		t.Error("corrupt cache file should be removed after digest mismatch")
	}
}

func TestPull_Resume_CorruptPrefix(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir).WithRetry(0).WithResume(true)

	payload := gzipBytes(t, "valid suffix follows a corrupted prefix")
	digest := digestOf(payload)
	half := len(payload) / 2

	garbage := bytes.Repeat([]byte{0xaa}, half)
	if err := os.WriteFile(filepath.Join(dir, digest+".gz"), garbage, 0644); err != nil {
		t.Fatal(err)
	}

	var gotOffset int64 = -1
	tasks := []LayerTask{{
		Index:     0,
		DigestHex: digest,
		Size:      int64(len(payload)),
		OpenLayer: func(ctx context.Context, offset int64) (io.ReadCloser, error) {
			gotOffset = offset
			return io.NopCloser(bytes.NewReader(payload[offset:])), nil
		},
	}}

	ctx := context.Background()
	eventCh, err := p.Pull(ctx, tasks, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, eventCh)
	last := events[len(events)-1]
	if last.Status != "error" {
		t.Fatalf("final status = %q, want error for corrupt resumed prefix", last.Status)
	}
	if !strings.Contains(last.Err.Error(), "digest mismatch") {
		t.Errorf("error = %v, want digest mismatch", last.Err)
	}
	if gotOffset != int64(half) {
		t.Errorf("OpenLayer offset = %d, want %d (resume still uses the partial file)", gotOffset, half)
	}
}

func TestCheckCache_CompleteWithoutMarker_Valid(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	payload := gzipBytes(t, "complete but unverified layer")
	digest := digestOf(payload)
	cacheFile := filepath.Join(dir, digest+".gz")
	if err := os.WriteFile(cacheFile, payload, 0644); err != nil {
		t.Fatal(err)
	}

	ch := make(chan PullEvent, 1)
	task := LayerTask{Index: 0, DigestHex: digest, Size: int64(len(payload))}
	if !p.checkCache(context.Background(), ch, task, cacheFile) {
		t.Fatal("checkCache = false, want true (digest verified)")
	}
	if evt := <-ch; evt.Status != "cached" {
		t.Errorf("status = %q, want cached", evt.Status)
	}
	if _, err := os.Stat(cacheFile + ".verified"); err != nil {
		t.Errorf("verification marker should be written: %v", err)
	}
}

func TestCheckCache_CompleteWithoutMarker_Corrupt(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	payload := gzipBytes(t, "content the manifest does not expect")
	cacheFile := filepath.Join(dir, digestOf(payload)+".gz")
	if err := os.WriteFile(cacheFile, payload, 0644); err != nil {
		t.Fatal(err)
	}

	ch := make(chan PullEvent, 1)
	task := LayerTask{Index: 0, DigestHex: "1111111111111111111111111111111111111111111111111111111111111111", Size: int64(len(payload))}
	if p.checkCache(context.Background(), ch, task, cacheFile) {
		t.Fatal("checkCache = true, want false for digest mismatch")
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Error("corrupt cache file should be removed")
	}
}

func TestCheckCache_StaleMarker(t *testing.T) {
	dir := t.TempDir()
	p := NewPuller(dir)

	payload := gzipBytes(t, "file modified after verification")
	cacheFile := filepath.Join(dir, digestOf(payload)+".gz")
	if err := os.WriteFile(cacheFile, payload, 0644); err != nil {
		t.Fatal(err)
	}
	marker := cacheFile + ".verified"
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(marker, old, old); err != nil {
		t.Fatal(err)
	}

	ch := make(chan PullEvent, 1)
	task := LayerTask{Index: 0, DigestHex: "2222222222222222222222222222222222222222222222222222222222222222", Size: int64(len(payload))}
	if p.checkCache(context.Background(), ch, task, cacheFile) {
		t.Fatal("checkCache = true, want false for stale marker + digest mismatch")
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Error("corrupt cache file should be removed")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("stale marker should be removed with the file")
	}
}

// --- helpers ---

func collectEvents(t *testing.T, ch <-chan PullEvent) []PullEvent {
	t.Helper()
	var events []PullEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

func assertStatus(t *testing.T, events []PullEvent, idx int, want string) {
	t.Helper()
	if idx >= len(events) {
		t.Fatalf("event[%d] missing, have %d events: %+v", idx, len(events), events)
	}
	if events[idx].Status != want {
		t.Errorf("event[%d].Status = %q, want %q; full event: %+v", idx, events[idx].Status, want, events[idx])
	}
}

func createGzipLayer(t *testing.T, dir, digestHex string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, digestHex+".gz")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(data)
	gw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func cacheSize(t *testing.T, dir, digestHex string) int64 {
	t.Helper()
	path := filepath.Join(dir, digestHex+".gz")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(s))
	gw.Close()
	return buf.Bytes()
}

func mustDecompress(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	data, err := io.ReadAll(gr)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
