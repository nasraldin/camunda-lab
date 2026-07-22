package trace

import (
	"context"
	"fmt"
	"time"
)

// Follow polls Get until completion, cancellation, timeout, idle stop, or max events.
// interval is the minimum wait between polls; values below minPollInterval are
// rejected to avoid busy-loops. Only changed timelines are emitted.
func (s *Service) Follow(
	ctx context.Context,
	request Request,
	interval time.Duration,
	emit func(Timeline) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if emit == nil {
		return fmt.Errorf("%w: follow emit callback is required", ErrInvalidRequest)
	}
	if interval < minPollInterval {
		return fmt.Errorf("%w: follow interval must be at least %s", ErrInvalidRequest, minPollInterval)
	}
	timeout := request.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < interval {
		return fmt.Errorf("%w: follow timeout must be at least the poll interval", ErrInvalidRequest)
	}
	maxEvents := request.MaxEvents
	if maxEvents == 0 {
		maxEvents = defaultMaxEvents
	}
	if maxEvents < 1 {
		return fmt.Errorf("%w: follow max events must be positive", ErrInvalidRequest)
	}
	if request.IdleStop < 0 {
		return fmt.Errorf("%w: follow idle stop must not be negative", ErrInvalidRequest)
	}

	deadline := Now().Add(timeout)
	prev, err := s.Get(ctx, request)
	if err != nil {
		return err
	}
	if err := emit(prev); err != nil {
		return err
	}
	emitted := 1
	if terminalState(prev.State) || emitted >= maxEvents {
		return nil
	}
	lastChange := Now()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := deadline.Sub(Now())
		if remaining <= 0 {
			return fmt.Errorf("follow timeout after %s", timeout)
		}
		wait := interval
		if wait > remaining {
			wait = remaining
		}
		if err := Wait(ctx, wait); err != nil {
			return err
		}
		if !Now().Before(deadline) {
			return fmt.Errorf("follow timeout after %s", timeout)
		}

		next, getErr := s.Get(ctx, request)
		if getErr != nil {
			return getErr
		}
		updated, changed := FollowOnce(prev, next)
		if changed {
			if err := emit(updated); err != nil {
				return err
			}
			emitted++
			prev = updated
			lastChange = Now()
			if terminalState(updated.State) || emitted >= maxEvents {
				return nil
			}
			continue
		}
		prev = next
		if terminalState(next.State) {
			return nil
		}
		if request.IdleStop > 0 && Now().Sub(lastChange) >= request.IdleStop {
			return nil
		}
	}
}

func terminalState(state string) bool {
	switch state {
	case "COMPLETED", "TERMINATED", "CANCELED", "CANCELLED":
		return true
	default:
		return false
	}
}
