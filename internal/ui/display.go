package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gitcode.com/DonaldTom/imgp/internal/puller"
)

var stderr = os.Stderr

type LayerState struct {
	Index   int
	Digest  string
	Total   int64
	Current int64
	Status  string
	ErrMsg  string
}

type ProgressDisplay struct {
	quiet    bool
	useANSI  bool
	mu       sync.Mutex
	Layers   []LayerState
	Total    int64
	HasError bool
}

func NewProgressDisplay(quiet bool) *ProgressDisplay {
	return &ProgressDisplay{quiet: quiet, useANSI: IsStderrTerminal()}
}

func (p *ProgressDisplay) GetLayers() []LayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	layers := make([]LayerState, len(p.Layers))
	copy(layers, p.Layers)
	return layers
}

func (p *ProgressDisplay) GetHasError() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.HasError
}

func (p *ProgressDisplay) runQuiet(ctx context.Context, eventCh <-chan puller.PullEvent, tasks []puller.LayerTask, quit chan<- struct{}) {
	p.initLayerStates(tasks)
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PANIC in runQuiet: %v\n", r)
		}
	}()
	defer close(quit)

	processEvent := func(evt puller.PullEvent) {
		p.mu.Lock()
		if evt.Err != nil {
			p.HasError = true
		}
		if evt.Index >= 0 && evt.Index < len(p.Layers) {
			p.Layers[evt.Index].Digest = evt.Digest
			p.Layers[evt.Index].Status = evt.Status
			if evt.Err != nil {
				p.Layers[evt.Index].Status = "error"
				p.Layers[evt.Index].ErrMsg = evt.Err.Error()
			}
		}
		p.mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			for evt := range eventCh {
				processEvent(evt)
			}
			return
		case evt, ok := <-eventCh:
			if !ok {
				return
			}
			processEvent(evt)
		}
	}
}

func (p *ProgressDisplay) initLayerStates(tasks []puller.LayerTask) {
	p.Layers = make([]LayerState, len(tasks))
	p.Total = 0
	for i, t := range tasks {
		p.Layers[i] = LayerState{Index: t.Index, Status: "pending"}
		p.Total += t.Size
	}
}

func (p *ProgressDisplay) startEventReader(ctx context.Context, eventCh <-chan puller.PullEvent) <-chan struct{} {
	readerDone := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "PANIC in display reader: %v\n", r)
			}
			close(readerDone)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-eventCh:
				if !ok {
					return
				}
				p.mu.Lock()
				if evt.Index >= 0 && evt.Index < len(p.Layers) {
					p.Layers[evt.Index].Digest = evt.Digest
					p.Layers[evt.Index].Total = evt.Total
					if evt.Err != nil {
						p.HasError = true
						p.Layers[evt.Index].Status = "error"
						p.Layers[evt.Index].ErrMsg = evt.Err.Error()
					} else {
						p.Layers[evt.Index].Current = evt.Bytes
						p.Layers[evt.Index].Status = evt.Status
					}
				}
				p.mu.Unlock()
			}
		}
	}()
	return readerDone
}

type frameStats struct {
	doneLayers   int
	currentBytes int64
	percent      float64
	allDone      bool
}

func (p *ProgressDisplay) calcProgress() frameStats {
	var s frameStats
	s.allDone = true
	for _, ls := range p.Layers {
		switch ls.Status {
		case "done", "cached":
			s.doneLayers++
			s.currentBytes += ls.Total
		case "downloading":
			s.allDone = false
			s.currentBytes += ls.Current
		default:
			s.allDone = false
		}
	}
	if p.Total > 0 {
		s.percent = float64(s.currentBytes) / float64(p.Total) * 100
	}
	return s
}

func (p *ProgressDisplay) renderFrame(totalLayers int) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	s := p.calcProgress()

	var buf strings.Builder
	fmt.Fprintf(&buf, "\033[2K  layers: [%d/%d] %.1f%% | %s / %s\n",
		s.doneLayers, totalLayers, s.percent,
		FormatBytes(s.currentBytes), FormatBytes(p.Total))

	for _, ls := range p.Layers {
		bar := RenderBar(ls.Current, ls.Total, 30)
		digest := Shorten(ls.Digest, 12)
		switch ls.Status {
		case "cached":
			fmt.Fprintf(&buf, "\033[2K    %s %s (cached)\n", "\u2713", digest)
		case "done":
			fmt.Fprintf(&buf, "\033[2K    %s %s %s\n", "\u2713", digest, bar)
		case "downloading":
			fmt.Fprintf(&buf, "\033[2K    %s %s %s %s/%s\n",
				"\u25CB", digest, bar,
				FormatBytes(ls.Current), FormatBytes(ls.Total))
		case "error":
			msg := "download failed"
			if ls.ErrMsg != "" {
				msg = ls.ErrMsg
			}
			fmt.Fprintf(&buf, "\033[2K    %s %s %s\n", "\u2717", digest, msg)
		default:
			fmt.Fprintf(&buf, "\033[2K    %s %s waiting...\n", "\u00B7", digest)
		}
	}
	return buf.String(), s.allDone
}

func (p *ProgressDisplay) RunPullUI(ctx context.Context, eventCh <-chan puller.PullEvent, tasks []puller.LayerTask) <-chan struct{} {
	quit := make(chan struct{})

	if p.quiet {
		go p.runQuiet(ctx, eventCh, tasks, quit)
		return quit
	}

	defer close(quit)

	p.initLayerStates(tasks)
	totalLayers := len(tasks)
	readerDone := p.startEventReader(ctx, eventCh)

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	prevLayers := 0
	for {
		select {
		case <-ctx.Done():
			<-readerDone
			return quit
		case <-ticker.C:
			if prevLayers > 0 && p.useANSI {
				fmt.Fprintf(stderr, "\033[%dA", prevLayers)
			}
			frame, allDone := p.renderFrame(totalLayers)
			if p.useANSI {
				prevLayers = 1 + totalLayers
			}
			fmt.Fprint(stderr, frame)

			if allDone {
				<-readerDone
				return quit
			}
		case <-readerDone:
			return quit
		}
	}
}

func Shorten(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func FormatBytes(b int64) string {
	if b < 0 {
		b = 0
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	switch exp {
	case 0:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(div))
	case 1:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(div))
	case 2:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(div))
	default:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(div))
	}
}

func RenderBar(current, total int64, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	if current > total {
		current = total
	}
	if current < 0 {
		current = 0
	}
	filled := int(float64(current) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	percent := float64(current) / float64(total) * 100
	return fmt.Sprintf("%s %.0f%%", bar, percent)
}

// Green wraps s in ANSI green escape codes.
func Green(s string) string { return "\033[32m" + s + "\033[0m" }

// Cyan wraps s in ANSI cyan escape codes.
func Cyan(s string) string { return "\033[36m" + s + "\033[0m" }

func IsTerminal() bool {
	return isTerminalFor(os.Stdout)
}

func IsStderrTerminal() bool {
	return isTerminalFor(os.Stderr)
}

func isTerminalFor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, _ := f.Stat()
	return fi != nil && (fi.Mode()&os.ModeCharDevice) != 0
}
