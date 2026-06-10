package runtimeutil

import "context"

func ContextWithCancelOnEither(parent, other context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if other == nil {
		return ctx, cancel
	}

	go func() {
		select {
		case <-other.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}
