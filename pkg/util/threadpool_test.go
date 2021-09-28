package util

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type poolTestObj struct {
	value int32
}

func (p *poolTestObj) increment() {
	atomic.AddInt32(&p.value, 1)
}

func (p *poolTestObj) get() int {
	return int(atomic.LoadInt32(&p.value))
}

func TestRunSync(t *testing.T) {
	pool := NewThreadPool(1)
	obj := &poolTestObj{}

	pool.RunSync(context.Background(), func() error {
		obj.increment()
		return nil
	})

	if got, want := obj.get(), 1; got != want {
		t.Errorf("Got %v, want %v", got, want)
	}
}

func TestBlockOnLimit(t *testing.T) {
	pool := NewThreadPool(2)
	obj := &poolTestObj{}

	for i := 0; i < 3; i++ {
		pool.Run(context.Background(), func() error {
			obj.increment()
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}

	if got, want := obj.get(), 2; got != want {
		t.Errorf("Got %v, want %v", got, want)
	}

	pool.Join(context.Background())

	if got, want := obj.get(), 3; got != want {
		t.Errorf("Got %v, want %v", got, want)
	}
}

func TestHighLoad(t *testing.T) {
	pool := NewThreadPool(20)
	obj := &poolTestObj{}

	for i := 0; i < 1000; i++ {
		pool.Run(context.Background(), func() error {
			obj.increment()
			return nil
		})
	}

	pool.Join(context.Background())

	if got, want := obj.get(), 1000; got != want {
		t.Errorf("Got %v, want %v", got, want)
	}
}

func TestRunError(t *testing.T) {
	pool := NewThreadPool(1)

	err := pool.Run(context.Background(), func() error {
		return fmt.Errorf("error!")
	})

	if err != nil {
		t.Errorf("Got %v, want no error", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = pool.Run(context.Background(), func() error { return nil })
	if err == nil {
		t.Errorf("Got %v, want error", err)
	}
}

func TestJoinError(t *testing.T) {
	pool := NewThreadPool(1)

	err := pool.Run(context.Background(), func() error {
		return fmt.Errorf("error!")
	})

	if err != nil {
		t.Errorf("Got %v, want no error", err)
	}

	err = pool.Join(context.Background())
	if err == nil {
		t.Errorf("Got %v, want error", err)
	}
}

func TestEndContext(t *testing.T) {
	pool := NewThreadPool(1)

	ctx, cancel := context.WithCancel(context.Background())

	err := pool.Run(ctx, func() error {
		time.Sleep(500 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("Got %v, want no error", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	// This blocks and then hits the sem.Acquire error.
	err = pool.Run(ctx, func() error { return nil })
	if err == nil {
		t.Errorf("Got %v, want error", err)
	}

	time.Sleep(500 * time.Millisecond)
	// This hits the ctx.Done() select case error.
	err = pool.Run(ctx, func() error { return nil })
	if err == nil {
		t.Errorf("Got %v, want error", err)
	}

	err = pool.Join(ctx)
	if err == nil {
		t.Errorf("Got %v, want error", err)
	}
}
