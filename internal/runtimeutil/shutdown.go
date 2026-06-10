package runtimeutil

import (
	"context"
	"sync"
)

type ShutdownCoordinator struct {
	mu            sync.Mutex
	accepting     bool
	active        int
	drained       chan struct{}
	drainedClosed bool
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewShutdownCoordinator() *ShutdownCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShutdownCoordinator{
		accepting: true,
		drained:   make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (c *ShutdownCoordinator) Begin() (done func(), ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.accepting {
		return func() {}, false
	}

	c.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()

			c.active--
			if c.active == 0 && !c.accepting {
				c.closeDrainedLocked()
			}
		})
	}, true
}

func (c *ShutdownCoordinator) StopAccepting() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.accepting = false
	if c.active == 0 {
		c.closeDrainedLocked()
	}
}

func (c *ShutdownCoordinator) Drain(ctx context.Context) error {
	c.StopAccepting()

	c.mu.Lock()
	drained := c.drained
	c.mu.Unlock()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *ShutdownCoordinator) ForceCancel() {
	c.cancel()
}

func (c *ShutdownCoordinator) Context() context.Context {
	return c.ctx
}

func (c *ShutdownCoordinator) Accepting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accepting
}

func (c *ShutdownCoordinator) IsDraining() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.accepting
}

func (c *ShutdownCoordinator) closeDrainedLocked() {
	if c.drainedClosed {
		return
	}
	close(c.drained)
	c.drainedClosed = true
}
