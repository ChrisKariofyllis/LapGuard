package notify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

type statusError struct {
	code int
}

func (e statusError) Error() string {
	return fmt.Sprintf("provider status %d", e.code)
}

func withRetry(ctx context.Context, attempts int, backoff time.Duration, fn func(context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	delay := backoff
	for i := 0; i < attempts; i++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = fn(ctx)
		if err == nil {
			return nil
		}
		if !retryable(err) || i == attempts-1 {
			return err
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			delay *= 2
		}
	}
	return err
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var se statusError
	if errors.As(err, &se) {
		return retryableStatus(se.code)
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func retryableStatus(code int) bool {
	return code == 429 || code >= 500
}
