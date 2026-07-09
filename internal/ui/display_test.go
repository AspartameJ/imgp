package ui

import (
	"context"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"gitcode.com/DonaldTom/imgp/internal/puller"
)

func TestShorten(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 3, "hel"},
		{"hi", 5, "hi"},
		{"", 3, ""},
		{"abc", 0, ""},
		{"abc", -1, ""},
	}
	for _, tt := range tests {
		got := Shorten(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("Shorten(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.b)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestRenderBar(t *testing.T) {
	bar := RenderBar(50, 100, 10)
	if !strings.Contains(bar, "50%") {
		t.Errorf("RenderBar(50, 100, 10) = %q, want 50%%", bar)
	}
	if len(bar) < 5 {
		t.Errorf("RenderBar returned too short: %q", bar)
	}

	zero := RenderBar(0, 0, 10)
	if zero != "░░░░░░░░░░" {
		t.Errorf("RenderBar(0, 0, 10) = %q, want empty bar", zero)
	}

	full := RenderBar(100, 100, 10)
	if !strings.Contains(full, "100%") {
		t.Errorf("RenderBar(100, 100, 10) = %q, want 100%%", full)
	}

	clamped := RenderBar(math.MaxInt64, 100, 10)
	if !strings.Contains(clamped, "100%") {
		t.Errorf("RenderBar overflow = %q, want clamped 100%%", clamped)
	}
}

func TestProgressDisplay_QuietMode(t *testing.T) {
	pd := NewProgressDisplay(true)

	eventCh := make(chan puller.PullEvent, 5)
	tasks := []puller.LayerTask{
		{Index: 0, DigestHex: "abc", Size: 100},
		{Index: 1, DigestHex: "def", Size: 200},
	}

	quit := pd.RunPullUI(context.Background(), eventCh, tasks)

	eventCh <- puller.PullEvent{Index: 0, Digest: "abc", Bytes: 100, Total: 100, Status: "done"}
	eventCh <- puller.PullEvent{Index: 1, Digest: "def", Status: "error", Err: io.ErrUnexpectedEOF}
	close(eventCh)
	<-quit

	if pd.HasError != true {
		t.Error("HasError should be true")
	}
	layers := pd.GetLayers()
	if len(layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(layers))
	}
	if layers[0].Status != "done" {
		t.Errorf("layer[0] status = %q, want done", layers[0].Status)
	}
	if layers[1].Status != "error" {
		t.Errorf("layer[1] status = %q, want error", layers[1].Status)
	}
}

func TestProgressDisplay_CollectWithCancel(t *testing.T) {
	pd := NewProgressDisplay(true)

	eventCh := make(chan puller.PullEvent, 2)
	tasks := []puller.LayerTask{
		{Index: 0, DigestHex: "abc", Size: 100},
	}

	quit := pd.RunPullUI(context.Background(), eventCh, tasks)
	close(eventCh)
	<-quit

	layers := pd.GetLayers()
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}
}

func TestCalcProgress(t *testing.T) {
	t.Run("all done", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "done", Total: 100},
				{Status: "done", Total: 200},
			},
			Total: 300,
		}
		s := pd.calcProgress()
		if !s.allDone {
			t.Error("expected allDone=true")
		}
		if s.doneLayers != 2 {
			t.Errorf("doneLayers=%d, want 2", s.doneLayers)
		}
		if s.currentBytes != 300 {
			t.Errorf("currentBytes=%d, want 300", s.currentBytes)
		}
		if s.percent != 100.0 {
			t.Errorf("percent=%f, want 100", s.percent)
		}
	})

	t.Run("cached treated as done", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "cached", Total: 50},
			},
			Total: 50,
		}
		s := pd.calcProgress()
		if !s.allDone {
			t.Error("expected allDone=true for cached")
		}
		if s.doneLayers != 1 {
			t.Errorf("doneLayers=%d, want 1", s.doneLayers)
		}
	})

	t.Run("downloading partial", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "downloading", Current: 30, Total: 100},
			},
			Total: 100,
		}
		s := pd.calcProgress()
		if s.allDone {
			t.Error("expected allDone=false for downloading")
		}
		if s.currentBytes != 30 {
			t.Errorf("currentBytes=%d, want 30", s.currentBytes)
		}
		if s.percent != 30.0 {
			t.Errorf("percent=%f, want 30", s.percent)
		}
	})

	t.Run("default status not done", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "pending"},
			},
			Total: 100,
		}
		s := pd.calcProgress()
		if s.allDone {
			t.Error("expected allDone=false for pending")
		}
		if s.doneLayers != 0 {
			t.Errorf("doneLayers=%d, want 0", s.doneLayers)
		}
	})

	t.Run("total zero does not divide", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "done", Total: 100},
			},
			Total: 0,
		}
		s := pd.calcProgress()
		if s.percent != 0.0 {
			t.Errorf("percent=%f, want 0 when Total=0", s.percent)
		}
	})

	t.Run("empty layers", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{},
			Total:  100,
		}
		s := pd.calcProgress()
		if !s.allDone {
			t.Error("expected allDone=true for empty layers")
		}
		if s.percent != 0.0 {
			t.Errorf("percent=%f, want 0", s.percent)
		}
	})

	t.Run("mixed statuses", func(t *testing.T) {
		pd := &ProgressDisplay{
			Layers: []LayerState{
				{Status: "done", Total: 100},
				{Status: "downloading", Current: 20, Total: 200},
				{Status: "pending"},
			},
			Total: 300,
		}
		s := pd.calcProgress()
		if s.allDone {
			t.Error("expected allDone=false with downloading/pending")
		}
		if s.doneLayers != 1 {
			t.Errorf("doneLayers=%d, want 1", s.doneLayers)
		}
		if s.currentBytes != 120 {
			t.Errorf("currentBytes=%d, want 120 (100 done + 20 downloading)", s.currentBytes)
		}
		if s.percent < 39.9 || s.percent > 40.1 {
			t.Errorf("percent=%f, want ~40", s.percent)
		}
	})
}

func TestRenderFrame_NonANSI(t *testing.T) {
	pd := &ProgressDisplay{
		useANSI: false,
		Layers: []LayerState{
			{Status: "done", Digest: "abc123", Total: 100, Current: 100},
			{Status: "downloading", Digest: "def456", Total: 200, Current: 50},
			{Status: "cached", Digest: "ghi789", Total: 300, Current: 300},
			{Status: "error", Digest: "err000", ErrMsg: "timeout"},
			{Status: "pending", Digest: "pend01"},
		},
		Total: 600,
	}
	frame, allDone := pd.renderFrame(5)
	if allDone {
		t.Error("expected allDone=false (downloading + pending)")
	}
	if !strings.Contains(frame, "layers: [") {
		t.Error("expected layers header in frame")
	}
	if !strings.Contains(frame, "75.0%") {
		t.Errorf("expected 75.0%% in frame, got: %q", frame)
	}
}

func TestRenderFrame_NonANSI_AllDone(t *testing.T) {
	pd := &ProgressDisplay{
		useANSI: false,
		Layers: []LayerState{
			{Status: "done", Digest: "abc123", Total: 100, Current: 100},
			{Status: "cached", Digest: "def456", Total: 200, Current: 200},
		},
		Total: 300,
	}
	_, allDone := pd.renderFrame(2)
	if !allDone {
		t.Error("expected allDone=true")
	}
}

func TestRenderFrame_ANSI(t *testing.T) {
	pd := &ProgressDisplay{
		useANSI: true,
		Layers: []LayerState{
			{Status: "done", Digest: "abc123", Total: 100, Current: 100},
			{Status: "downloading", Digest: "def456", Total: 200, Current: 50},
			{Status: "cached", Digest: "ghi789", Total: 300, Current: 300},
			{Status: "error", Digest: "err000", ErrMsg: "timeout"},
			{Status: "pending", Digest: "pend01"},
		},
		Total: 600,
	}
	frame, allDone := pd.renderFrame(5)
	if allDone {
		t.Error("expected allDone=false")
	}
	if !strings.Contains(frame, "75.0%") {
		t.Errorf("expected 75.0%% in frame, got: %q", frame)
	}
	if !strings.Contains(frame, "\u2713") {
		t.Error("expected check mark in frame")
	}
}

func TestRenderFrame_ANSI_EmptyErrMsg(t *testing.T) {
	pd := &ProgressDisplay{
		useANSI: true,
		Layers: []LayerState{
			{Status: "error", Digest: "err000"},
		},
		Total: 100,
	}
	frame, allDone := pd.renderFrame(1)
	if allDone {
		t.Error("expected allDone=false")
	}
	if !strings.Contains(frame, "download failed") {
		t.Errorf("expected default 'download failed' message, got: %q", frame)
	}
}

func TestGetHasError(t *testing.T) {
	pd := &ProgressDisplay{HasError: true}
	if !pd.GetHasError() {
		t.Error("expected GetHasError()=true")
	}
	pd2 := &ProgressDisplay{HasError: false}
	if pd2.GetHasError() {
		t.Error("expected GetHasError()=false")
	}
}

func TestColorFunctions(t *testing.T) {
	if Green("ok") != "\033[32mok\033[0m" {
		t.Errorf("Green = %q", Green("ok"))
	}
	if Cyan("info") != "\033[36minfo\033[0m" {
		t.Errorf("Cyan = %q", Cyan("info"))
	}
}

func TestIsTerminal(t *testing.T) {
	// In CI/test environments, stdout is typically not a terminal.
	// Just verify it doesn't panic and returns a bool.
	got := IsTerminal()
	if _, ok := interface{}(got).(bool); !ok {
		t.Error("IsTerminal() should return bool")
	}
}

func TestNewProgressDisplay(t *testing.T) {
	pd := NewProgressDisplay(true)
	if !pd.quiet {
		t.Error("expected quiet mode")
	}
	pd2 := NewProgressDisplay(false)
	if pd2.quiet {
		t.Error("expected non-quiet mode")
	}
}

func TestProgressDisplay_ActiveMode_Cancel(t *testing.T) {
	pd := NewProgressDisplay(false)
	eventCh := make(chan puller.PullEvent, 5)
	tasks := []puller.LayerTask{
		{Index: 0, DigestHex: "abc", Size: 100},
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		eventCh <- puller.PullEvent{Index: 0, Digest: "abc", Bytes: 50, Total: 100, Status: "downloading"}
		time.Sleep(50 * time.Millisecond)
		cancel()
		close(eventCh)
	}()

	done := make(chan struct{})
	go func() {
		quit := pd.RunPullUI(ctx, eventCh, tasks)
		<-quit
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunPullUI did not return after cancel")
	}
}

func TestProgressDisplay_ActiveMode(t *testing.T) {
	pd := NewProgressDisplay(false)

	eventCh := make(chan puller.PullEvent, 5)
	tasks := []puller.LayerTask{
		{Index: 0, DigestHex: "abc", Size: 100},
	}

	go func() {
		eventCh <- puller.PullEvent{Index: 0, Digest: "abc", Bytes: 50, Total: 100, Status: "downloading"}
		eventCh <- puller.PullEvent{Index: 0, Digest: "abc", Bytes: 100, Total: 100, Status: "done"}
		close(eventCh)
	}()

	quit := pd.RunPullUI(context.Background(), eventCh, tasks)
	<-quit

	layers := pd.GetLayers()
	if len(layers) != 1 {
		t.Fatalf("got %d layers, want 1", len(layers))
	}
	if layers[0].Status != "done" {
		t.Errorf("layer[0] status = %q, want done", layers[0].Status)
	}
}
