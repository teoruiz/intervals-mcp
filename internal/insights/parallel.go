package insights

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// gather runs every task concurrently and waits for all of them.
//
// The first failure cancels the context handed to the others, so one broken
// call does not leave its siblings running. Errors are joined in task order
// rather than completion order, which keeps the message stable no matter which
// call happens to fail first.
func gather(ctx context.Context, tasks ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make([]error, len(tasks))
	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Go(func() {
			if err := task(ctx); err != nil {
				errs[i] = err
				cancel()
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// fetch adapts a single API call into a gather task. Each task owns its
// destination, so concurrent tasks never write the same memory and no lock is
// needed. label prefixes any error, matching the wrapping the sequential code
// used to do at the call site.
func fetch[T any](dst *T, label string, call func(context.Context) (T, error)) func(context.Context) error {
	return func(ctx context.Context) error {
		value, err := call(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		*dst = value
		return nil
	}
}
