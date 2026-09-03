package insights

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestGatherRunsTasksConcurrently uses a barrier: every task blocks until all
// tasks have started. Sequential execution can never satisfy it, so the test
// fails by timing out rather than by passing accidentally.
func TestGatherRunsTasksConcurrently(t *testing.T) {
	const count = 4
	var started sync.WaitGroup
	started.Add(count)
	release := make(chan struct{})

	tasks := make([]func(context.Context) error, count)
	for i := range tasks {
		tasks[i] = func(context.Context) error {
			started.Done()
			select {
			case <-release:
				return nil
			case <-time.After(5 * time.Second):
				return errors.New("timed out waiting for siblings to start")
			}
		}
	}

	done := make(chan error, 1)
	go func() { done <- gather(context.Background(), tasks...) }()

	started.Wait() // only reachable if all tasks run at once
	close(release)

	if err := <-done; err != nil {
		t.Fatalf("gather: %v", err)
	}
}

func TestGatherReturnsNilWhenEveryTaskSucceeds(t *testing.T) {
	var count int
	var mu sync.Mutex
	task := func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		count++
		return nil
	}
	if err := gather(context.Background(), task, task, task); err != nil {
		t.Fatalf("gather: %v", err)
	}
	if count != 3 {
		t.Fatalf("tasks run = %d, want 3", count)
	}
}

// Errors must be joined in task order, not completion order, so the message
// does not depend on which call happens to lose the race.
func TestGatherJoinsErrorsInTaskOrder(t *testing.T) {
	slow := func(context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return errors.New("first task")
	}
	fast := func(context.Context) error {
		return errors.New("third task")
	}

	err := gather(context.Background(), slow, func(context.Context) error { return nil }, fast)
	if err == nil {
		t.Fatal("expected an error")
	}
	message := err.Error()
	firstAt := strings.Index(message, "first task")
	thirdAt := strings.Index(message, "third task")
	if firstAt < 0 || thirdAt < 0 {
		t.Fatalf("message missing an error: %q", message)
	}
	if firstAt > thirdAt {
		t.Fatalf("errors joined in completion order, not task order: %q", message)
	}
}

func TestGatherCancelsSiblingsOnFirstFailure(t *testing.T) {
	failed := make(chan struct{})
	var siblingErr error

	err := gather(context.Background(),
		func(context.Context) error {
			close(failed)
			return errors.New("boom")
		},
		func(ctx context.Context) error {
			<-failed
			select {
			case <-ctx.Done():
				siblingErr = ctx.Err()
			case <-time.After(5 * time.Second):
				siblingErr = errors.New("sibling was never cancelled")
			}
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	if !errors.Is(siblingErr, context.Canceled) {
		t.Fatalf("sibling context error = %v, want context.Canceled", siblingErr)
	}
}

func TestFetchAssignsResultAndLabelsErrors(t *testing.T) {
	var got string
	task := fetch(&got, "load thing", func(context.Context) (string, error) {
		return "value", nil
	})
	if err := task(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "value" {
		t.Fatalf("got = %q", got)
	}

	sentinel := errors.New("upstream")
	failing := fetch(&got, "load thing", func(context.Context) (string, error) {
		return "", sentinel
	})
	err := failing(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("error does not wrap the cause: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "load thing: ") {
		t.Fatalf("error missing label: %v", err)
	}
	if got != "value" {
		t.Fatalf("destination overwritten on failure: %q", got)
	}
}
