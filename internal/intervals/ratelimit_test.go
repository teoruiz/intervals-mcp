package intervals

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func rateLimitedResponse(retryAfter string) *http.Response {
	res := jsonResponse(429, `{"error":"rate limited"}`)
	if retryAfter != "" {
		res.Header.Set("Retry-After", retryAfter)
	}
	return res
}

func newTestClient(t *testing.T, cfg Config, rt roundTripFunc) *Client {
	t.Helper()
	cfg.BaseURL = "https://intervals.test"
	cfg.APIKey = "secret"
	cfg.AthleteID = "i123"
	cfg.HTTPClient = &http.Client{Transport: rt}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// A short Retry-After means the per-second IP limit, which is worth waiting out.
func TestGetRetriesShortRateLimitWait(t *testing.T) {
	var calls atomic.Int64
	client := newTestClient(t, Config{AthleteCacheTTL: -1}, func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return rateLimitedResponse("1"), nil
		}
		return jsonResponse(200, `{"id":"i123","timezone":"UTC"}`), nil
	})

	athlete, err := client.GetAthlete(context.Background())
	if err != nil {
		t.Fatalf("GetAthlete: %v", err)
	}
	if athlete.Timezone != "UTC" {
		t.Fatalf("Timezone = %q", athlete.Timezone)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 (one rate limited, one retry)", got)
	}
}

// A long Retry-After is the 15 minute or daily bucket. Waiting would blow the
// request timeout, so it must surface immediately and carry the wait.
func TestGetFailsFastOnLongRateLimitWait(t *testing.T) {
	var calls atomic.Int64
	client := newTestClient(t, Config{AthleteCacheTTL: -1}, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return rateLimitedResponse("370"), nil
	})

	_, err := client.GetAthlete(context.Background())
	var limited *RateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("error = %v, want *RateLimitError", err)
	}
	if limited.RetryAfter != 370*time.Second {
		t.Fatalf("RetryAfter = %s, want 370s", limited.RetryAfter)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1 (no retry on a long wait)", got)
	}
	if !strings.Contains(limited.Error(), "retry after") {
		t.Fatalf("message not actionable: %q", limited.Error())
	}
}

func TestGetGivesUpAfterRepeatedShortRateLimits(t *testing.T) {
	var calls atomic.Int64
	client := newTestClient(t, Config{AthleteCacheTTL: -1}, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return rateLimitedResponse("0"), nil
	})

	_, err := client.GetAthlete(context.Background())
	var limited *RateLimitError
	if !errors.As(err, &limited) {
		t.Fatalf("error = %v, want *RateLimitError", err)
	}
	if got := calls.Load(); got != maxRateLimitAttempts {
		t.Fatalf("requests = %d, want %d", got, maxRateLimitAttempts)
	}
}

func TestParseRetryAfterAcceptsSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"370", 370 * time.Second},
		{"0", 0},
		{"", 0},
		{"-5", 0},
		{"nonsense", 0},
		{now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}
	for _, tc := range cases {
		if got := parseRetryAfter(tc.value, now); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %s, want %s", tc.value, got, tc.want)
		}
	}
}

// The per-second ceiling is invisible in headers, so the cap is the defence.
func TestClientCapsConcurrentRequests(t *testing.T) {
	const cap = 3
	var inFlight, peak atomic.Int64
	var release sync.WaitGroup
	release.Add(1)

	client := newTestClient(t, Config{AthleteCacheTTL: -1, MaxConcurrentRequests: cap},
		func(*http.Request) (*http.Response, error) {
			current := inFlight.Add(1)
			for {
				old := peak.Load()
				if current <= old || peak.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
			return jsonResponse(200, `{"id":"i123"}`), nil
		})

	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			if _, err := client.GetAthlete(context.Background()); err != nil {
				t.Errorf("GetAthlete: %v", err)
			}
		})
	}
	wg.Wait()

	if got := peak.Load(); got > cap {
		t.Fatalf("peak concurrent requests = %d, want at most %d", got, cap)
	}
	if peak.Load() < 2 {
		t.Fatalf("requests never overlapped, cap is untested: peak = %d", peak.Load())
	}
}

func TestClientRecordsAndReportsRateLimitBudget(t *testing.T) {
	client := newTestClient(t, Config{AthleteCacheTTL: -1}, func(*http.Request) (*http.Response, error) {
		res := jsonResponse(200, `{"id":"i123"}`)
		res.Header.Set("X-RateLimit-Limit", "500,10000")
		res.Header.Set("X-RateLimit-Remaining", "499,9000")
		return res, nil
	})

	if _, err := client.GetAthlete(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := client.RateLimit()
	if !got.Observed {
		t.Fatal("rate limit was not observed")
	}
	if got.FifteenMinuteLimit != 500 || got.FifteenMinuteRemaining != 499 {
		t.Fatalf("window = %d/%d", got.FifteenMinuteRemaining, got.FifteenMinuteLimit)
	}
	if got.DailyLimit != 10000 || got.DailyRemaining != 9000 {
		t.Fatalf("daily = %d/%d", got.DailyRemaining, got.DailyLimit)
	}
}

func TestClientLogsBudgetOnceThenWarnsWhenLow(t *testing.T) {
	var buf bytes.Buffer
	var remaining atomic.Int64
	remaining.Store(400)
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	client := newTestClient(t, Config{AthleteCacheTTL: -1, Logger: logger},
		func(*http.Request) (*http.Response, error) {
			res := jsonResponse(200, `{"id":"i123"}`)
			res.Header.Set("X-RateLimit-Limit", "500,10000")
			res.Header.Set("X-RateLimit-Remaining", strconv.FormatInt(remaining.Load(), 10)+",9000")
			return res, nil
		})

	for range 3 {
		if _, err := client.GetAthlete(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Count(buf.String(), "rate limit budget"); got != 1 {
		t.Fatalf("healthy budget logged %d times, want 1", got)
	}

	remaining.Store(5)
	if _, err := client.GetAthlete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "running low") {
		t.Fatalf("low budget did not warn: %s", buf.String())
	}
}

func TestParseRateLimitPairRejectsMalformedHeaders(t *testing.T) {
	for _, value := range []string{"", "500", "a,b", "500,", ",10000"} {
		if _, ok := parseRateLimitPair(value); ok {
			t.Errorf("parseRateLimitPair(%q) accepted a malformed header", value)
		}
	}
	got, ok := parseRateLimitPair(" 500 , 10000 ")
	if !ok || got != [2]int{500, 10000} {
		t.Fatalf("parseRateLimitPair = %v, %v", got, ok)
	}
}
