package longpoll

import (
	"context"
	"io"
	"time"
)

// ChannelConfig models optional configuration for the Channel function.
type ChannelConfig struct {
	// MaxSize is the absolute maximum number of values to receive. Setting
	// this to a value < 0 will disable the maximum size constraint.
	//
	// Defaults to 16, if 0.
	MaxSize int

	// MinSize is the (target) minimum number of values to receive. If
	// PartialTimeout is configured, the effective minimum size will be 1, if
	// the PartialTimeout is reached.
	//
	// Setting this to a value < 0 will cause the PartialTimeout to start from
	// the call to Channel, and will allow returning without receiving any
	// values. In this scenario, PartialTimeout will apply to the first value.
	//
	// Defaults to 4, if 0.
	MinSize int

	// PartialTimeout is the maximum time to wait for a partial response,
	// defined as a number of received values less than the MinSize. After/if
	// this timeout is reached, the effective minimum size will be reduced, see
	// MinSize for details.
	//
	// Defaults to 50ms, if 0.
	PartialTimeout time.Duration
}

// Channel performs a blocking receive on the channel, returning as many values
// as possible, given the constraints. If ctx cancels, the error will be
// returned. The cfg parameter is optional, and may be nil, in which case the
// documented defaults will be used. Values will be received from ch, and
// passed to handler. Errors from handler will be returned, and cause the call
// to Channel to return.
//
// If the channel is closed, and all buffered values are received, Channel will
// return io.EOF. In this scenario, the minimum size may not be reached.
//
// Providing a nil ctx, ch, or handler will cause a panic.
func Channel[T any](ctx context.Context, cfg *ChannelConfig, ch <-chan T, handler func(value T) error) error {
	if ctx == nil {
		panic(`longpoll: nil context`)
	}
	if ch == nil {
		panic(`longpoll: nil channel`)
	}
	if handler == nil {
		panic(`longpoll: nil handler`)
	}

	// guard context cancel - nice to have consistent behavior (avoid receive if canceled)
	if err := ctx.Err(); err != nil {
		return err
	}

	maxSize := 16
	minSize := 4
	partialTimeout := 50 * time.Millisecond
	if cfg != nil {
		if cfg.MaxSize != 0 {
			maxSize = cfg.MaxSize
		}
		if cfg.MinSize != 0 {
			minSize = cfg.MinSize
		}
		if cfg.PartialTimeout != 0 {
			partialTimeout = cfg.PartialTimeout
		}
	}

	var partialTimeoutCh <-chan time.Time
	if partialTimeout > 0 && minSize < 0 {
		// we have a partial timeout, but no minimum size - special case, starts the timeout immediately
		timer := time.NewTimer(partialTimeout)
		defer timer.Stop()
		partialTimeoutCh = timer.C
	}

	var size int

	// receive the minimum number of values (or first value) OR partial timeout OR context cancel
MinSizeLoop:
	for (maxSize < 0 || size < maxSize) && (size < minSize || (size == 0 && partialTimeoutCh != nil)) {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-partialTimeoutCh:
			if err := ctx.Err(); err != nil {
				return err
			}
			break MinSizeLoop

		case value, ok := <-ch:
			if !ok {
				return io.EOF
			}

			size++

			if size == 1 && partialTimeout > 0 && partialTimeoutCh == nil {
				// first value received, start the partial timeout
				timer := time.NewTimer(partialTimeout)
				//goland:noinspection GoDeferInLoop
				defer timer.Stop()
				partialTimeoutCh = timer.C
			}

			if err := handler(value); err != nil {
				return err
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}
	}

	// receive what additional values we can, up to the maximum size OR context cancel
MaxSizeLoop:
	for maxSize < 0 || size < maxSize {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case value, ok := <-ch:
			if !ok {
				return io.EOF
			}

			size++

			if err := handler(value); err != nil {
				return err
			}

		default:
			if err := ctx.Err(); err != nil {
				return err
			}
			break MaxSizeLoop
		}

		if err := ctx.Err(); err != nil {
			return err
		}
	}

	return nil
}
