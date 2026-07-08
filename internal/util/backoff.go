package util

import (
	"context"
	"time"
)

func Backoff(ctx context.Context, attempt int) error {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	const maxShift = 30
	if shift > maxShift {
		shift = maxShift
	}
	d := time.Duration(1<<uint(shift)) * time.Second
	const maxBackoff = 30 * time.Second
	if d > maxBackoff {
		d = maxBackoff
	}
	timer := time.NewTimer(d)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
