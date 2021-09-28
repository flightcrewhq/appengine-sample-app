package util

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"
)

type ThreadPool struct {
	// Limits the number of threads.
	sem *semaphore.Weighted
	// Can wait for the outstanding threads to finish.
	wg *sync.WaitGroup
	// Catches errors from async executions.
	ec chan error
}

func NewThreadPool(n int) *ThreadPool {
	t := &ThreadPool{
		sem: semaphore.NewWeighted(int64(n)),
		wg:  new(sync.WaitGroup),
		ec:  make(chan error),
	}

	return t
}

func (t *ThreadPool) Run(ctx context.Context, f func() error) error {
	// In case all possible threads are blocked trying to return an error before
	// Join() is called, return the error here to avoid blocking forever on
	// acquire().
	select {
	case err := <-t.ec:
		return fmt.Errorf("caught async error: %v", err)
	default:
		// Carry on.
	}

	if err := t.acquire(ctx); err != nil {
		return err
	}

	go func() {
		defer t.release()
		if err := f(); err != nil {
			t.ec <- fmt.Errorf("async function execution error: %v", err)
		}
	}()

	return nil
}

// Run the function in the same thread and wait for it to finish.
// This is useful when the caller is already in a goroutine (for example,
// handling an HTTP request).
func (t *ThreadPool) RunSync(ctx context.Context, f func() error) error {
	if err := t.acquire(ctx); err != nil {
		return err
	}
	defer t.release()

	return f()
}

func (t *ThreadPool) Join(ctx context.Context) error {
	c := make(chan struct{})
	go func() {
		defer close(c)
		t.wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case err := <-t.ec:
		return fmt.Errorf("caught async error: %v", err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *ThreadPool) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		// From sem.Acquire() docs:
		// "If ctx is already done, Acquire may still succeed without blocking."
		// So, ensure that an ended context doesn't succeed here.
		return ctx.Err()
	default:
		// Don't block.
	}

	if err := t.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	t.wg.Add(1)
	return nil
}

func (t *ThreadPool) release() {
	t.wg.Done()
	t.sem.Release(1)
}
