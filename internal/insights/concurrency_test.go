package insights

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

// barrierClient blocks each of the four today_context calls until all of them
// have arrived. If the service issued them sequentially the barrier could
// never fill and the test would fail on the timeout instead of hanging.
type barrierClient struct {
	arrived sync.WaitGroup
	release chan struct{}
	timeout error
	mu      sync.Mutex
}

func newBarrierClient(expected int) *barrierClient {
	c := &barrierClient{release: make(chan struct{})}
	c.arrived.Add(expected)
	go func() {
		c.arrived.Wait()
		close(c.release)
	}()
	return c
}

func (c *barrierClient) wait() {
	c.arrived.Done()
	select {
	case <-c.release:
	case <-time.After(5 * time.Second):
		c.mu.Lock()
		c.timeout = errors.New("calls were issued sequentially")
		c.mu.Unlock()
	}
}

func (c *barrierClient) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timeout
}

func (c *barrierClient) GetAthlete(context.Context) (*intervals.Athlete, error) {
	// Not part of the fan-out: the date depends on it, so it stays first.
	return &intervals.Athlete{ID: "i123", Timezone: "UTC"}, nil
}

func (c *barrierClient) ListActivities(context.Context, string, string, int) ([]intervals.Activity, error) {
	c.wait()
	return []intervals.Activity{{ID: "a1", Name: "Ride"}}, nil
}

func (c *barrierClient) GetWellness(context.Context, string) (*intervals.Wellness, error) {
	c.wait()
	return &intervals.Wellness{}, nil
}

func (c *barrierClient) GetAthleteSummary(context.Context, string, string) ([]intervals.Summary, error) {
	c.wait()
	return []intervals.Summary{{}}, nil
}

func (c *barrierClient) ListEvents(context.Context, string, string, []string, int) ([]intervals.Event, error) {
	c.wait()
	id := 1
	return []intervals.Event{{ID: &id}}, nil
}

func (c *barrierClient) GetActivity(context.Context, string, bool) (*intervals.Activity, error) {
	c.wait()
	return &intervals.Activity{ID: "a1"}, nil
}

func (c *barrierClient) GetActivityStreams(context.Context, string, []string) ([]intervals.ActivityStream, error) {
	c.wait()
	return nil, nil
}

func (c *barrierClient) ListWellness(context.Context, string, string) ([]intervals.Wellness, error) {
	return nil, nil
}

func (c *barrierClient) GetEvent(context.Context, int) (*intervals.Event, error) {
	return nil, nil
}

func TestTodayContextFetchesConcurrently(t *testing.T) {
	client := newBarrierClient(4)
	service := New(client)

	result, err := service.TodayContext(context.Background(), TodayArgs{Date: "2026-09-03"})
	if err != nil {
		t.Fatalf("TodayContext: %v", err)
	}
	if err := client.err(); err != nil {
		t.Fatal(err)
	}
	if len(result.Activities) != 1 || result.Summary == nil || len(result.PlannedEvents) != 1 {
		t.Fatalf("result lost data: %+v", result)
	}
}

func TestSearchFetchesConcurrently(t *testing.T) {
	client := newBarrierClient(3)
	service := New(client)

	if _, err := service.Search(context.Background(), SearchArgs{Query: "ride"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if err := client.err(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryFetchesConcurrently(t *testing.T) {
	client := newBarrierClient(2)
	service := New(client)

	if _, err := service.Recovery(context.Background(), RecoveryArgs{Date: "2026-09-03"}); err != nil {
		t.Fatalf("Recovery: %v", err)
	}
	if err := client.err(); err != nil {
		t.Fatal(err)
	}
}

func TestActivityFetchesStreamsConcurrentlyWithActivity(t *testing.T) {
	client := newBarrierClient(2)
	service := New(client)

	detail, err := service.Activity(context.Background(), ActivityArgs{
		ID:                     "a1",
		IncludeRunningDynamics: true,
	})
	if err != nil {
		t.Fatalf("Activity: %v", err)
	}
	if err := client.err(); err != nil {
		t.Fatal(err)
	}
	if detail.Activity == nil || detail.RunningDynamics == nil {
		t.Fatalf("detail incomplete: %+v", detail)
	}
}
