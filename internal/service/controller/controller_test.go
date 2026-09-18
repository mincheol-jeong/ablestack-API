package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
)

func TestControllerPreventsOverlappingHandlerRuns(t *testing.T) {
	c := &TypeController{interval: 5 * time.Millisecond}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32

	c.StatusRegister(func() {
		calls.Add(1)
		current := active.Add(1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		active.Add(-1)
	})

	startReturned := make(chan struct{})
	go func() {
		c.Start()
		close(startReturned)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	time.Sleep(25 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("overlapping handler was started: calls=%d", got)
	}

	close(release)
	c.Stop()
	select {
	case <-startReturned:
	case <-time.After(time.Second):
		t.Fatal("controller did not stop")
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("handler overlap detected: max active=%d", got)
	}
}

func TestControllerErrorAccessIsConcurrentSafe(t *testing.T) {
	c := &TypeController{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			c.AddError(errors.New("test error"))
		}()
		go func() {
			defer wg.Done()
			_ = c.GetError()
		}()
		go func() {
			defer wg.Done()
			c.ClearError()
		}()
	}
	wg.Wait()

	copyOfErrors := c.GetError()
	copyOfErrors.Errors = append(copyOfErrors.Errors, utils.Errorlog{Error: "external mutation"})
	if len(c.GetError().Errors) == len(copyOfErrors.Errors) {
		t.Fatal("GetError returned mutable controller state")
	}
}
