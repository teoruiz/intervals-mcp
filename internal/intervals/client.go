package intervals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultAthleteCacheTTL bounds how long the athlete profile is reused. Nearly
// every tool resolves "today" through the athlete's timezone, so without this
// each one pays a round trip for a value that almost never changes. The TTL is
// short because `intervals mcp stdio` can run for days on a laptop.
const defaultAthleteCacheTTL = 10 * time.Minute

const (
	// Intervals.icu allows 10 calls per second per IP on top of its 15 minute
	// and daily budgets. That per-second ceiling returns no headers, so the
	// only defence is not exceeding it: cap how many calls are ever in flight.
	// The egress IP belongs to the container platform, not to us.
	defaultMaxConcurrentRequests = 4

	// Only a short Retry-After is worth waiting out inside a request. A small
	// value means the per-second limit. The 15 minute and daily buckets report
	// values far beyond any sane request timeout, so those fail fast instead.
	maxRateLimitAttempts = 3
	maxRateLimitSleep    = 2 * time.Second

	// Warn once the remaining budget falls below this share of the limit.
	rateLimitWarnFraction = 0.1
)

// RateLimitError reports a 429 from Intervals.icu together with the wait it
// asked for, so a tool can tell the caller when to come back instead of
// surfacing an opaque status code.
type RateLimitError struct {
	RetryAfter time.Duration
	Body       string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("intervals API rate limited, retry after %s", e.RetryAfter)
	}
	return "intervals API rate limited"
}

// RateLimit is the budget Intervals.icu reports on every response, as
// "X-RateLimit-Limit: <15m>,<daily>" and the matching Remaining header. The
// per-second IP limit is not reported and is absent here.
type RateLimit struct {
	Observed               bool
	FifteenMinuteLimit     int
	FifteenMinuteRemaining int
	DailyLimit             int
	DailyRemaining         int
}

var ErrNotFound = errors.New("intervals resource not found")

type Client struct {
	baseURL   *url.URL
	apiKey    string
	athleteID string
	http      *http.Client
	timeout   time.Duration

	athleteTTL time.Duration
	now        func() time.Time

	athleteMu        sync.Mutex
	athlete          *Athlete
	athleteFetchedAt time.Time

	// sem caps calls in flight. nil means unlimited.
	sem    chan struct{}
	logger *slog.Logger

	rateMu    sync.Mutex
	rateLimit RateLimit
}

type Config struct {
	BaseURL    string
	APIKey     string
	AthleteID  string
	HTTPClient *http.Client
	Timeout    time.Duration

	// AthleteCacheTTL overrides how long GetAthlete reuses a cached profile.
	// Zero selects defaultAthleteCacheTTL; a negative value disables caching.
	AthleteCacheTTL time.Duration

	// Now overrides the clock used to expire the athlete cache, for tests.
	Now func() time.Time

	// MaxConcurrentRequests caps how many calls are in flight at once. Zero
	// selects defaultMaxConcurrentRequests; a negative value removes the cap.
	MaxConcurrentRequests int

	// Logger, when set, reports the rate limit budget: once when first seen,
	// and again whenever the remaining allowance runs low.
	Logger *slog.Logger
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("intervals API status %d: %s", e.StatusCode, e.Body)
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("base URL is required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("API key is required")
	}
	if cfg.AthleteID == "" {
		return nil, errors.New("athlete ID is required")
	}
	baseURL, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	athleteTTL := cfg.AthleteCacheTTL
	if athleteTTL == 0 {
		athleteTTL = defaultAthleteCacheTTL
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	concurrency := cfg.MaxConcurrentRequests
	if concurrency == 0 {
		concurrency = defaultMaxConcurrentRequests
	}
	var sem chan struct{}
	if concurrency > 0 {
		sem = make(chan struct{}, concurrency)
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		athleteID:  cfg.AthleteID,
		http:       client,
		timeout:    timeout,
		athleteTTL: athleteTTL,
		now:        now,
		sem:        sem,
		logger:     cfg.Logger,
	}, nil
}

// RateLimit returns the most recent budget reported by Intervals.icu.
func (c *Client) RateLimit() RateLimit {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	return c.rateLimit
}

func (c *Client) AthleteID() string {
	return c.athleteID
}

// GetAthlete returns the athlete profile, reusing a cached copy for
// AthleteCacheTTL. The lock is held across the fetch so that concurrent
// callers collapse into one request instead of stampeding the API. Callers get
// a copy, so a cached profile can never be mutated through a returned pointer.
func (c *Client) GetAthlete(ctx context.Context) (*Athlete, error) {
	if c.athleteTTL < 0 {
		return c.fetchAthlete(ctx)
	}

	c.athleteMu.Lock()
	defer c.athleteMu.Unlock()
	if c.athlete != nil && c.now().Sub(c.athleteFetchedAt) < c.athleteTTL {
		cached := *c.athlete
		return &cached, nil
	}
	athlete, err := c.fetchAthlete(ctx)
	if err != nil {
		return nil, err
	}
	c.athlete = athlete
	c.athleteFetchedAt = c.now()
	cached := *athlete
	return &cached, nil
}

func (c *Client) fetchAthlete(ctx context.Context) (*Athlete, error) {
	var athlete Athlete
	if err := c.get(ctx, c.athletePath(), nil, &athlete); err != nil {
		return nil, err
	}
	return &athlete, nil
}

func (c *Client) ListActivities(ctx context.Context, oldest, newest string, limit int) ([]Activity, error) {
	values := url.Values{}
	values.Set("oldest", oldest)
	if newest != "" {
		values.Set("newest", newest)
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	values.Set("fields", strings.Join(activityListFields, ","))

	var activities []Activity
	if err := c.get(ctx, c.athletePath("activities"), values, &activities); err != nil {
		return nil, err
	}
	for i := range activities {
		normalizeActivity(&activities[i])
	}
	return activities, nil
}

// normalizeActivity converts raw Intervals values into the units the rest of
// the app uses. Intervals stores running cadence per leg; double it to steps
// per minute, the standard running measure shown by watches and platforms.
func normalizeActivity(a *Activity) {
	if a == nil {
		return
	}
	if a.AverageCadence != nil && IsRunType(a.Type) {
		spm := *a.AverageCadence * 2
		a.AverageCadence = &spm
	}
}

// IsRunType reports whether an activity type is a run (Run, TrailRun, VirtualRun, etc.).
func IsRunType(activityType string) bool {
	return strings.Contains(strings.ToLower(activityType), "run")
}

func (c *Client) GetActivity(ctx context.Context, id string, includeIntervals bool) (*Activity, error) {
	values := url.Values{}
	if includeIntervals {
		values.Set("intervals", "true")
	}
	var activity Activity
	if err := c.get(ctx, "/api/v1/activity/"+url.PathEscape(id), values, &activity); err != nil {
		return nil, err
	}
	normalizeActivity(&activity)
	return &activity, nil
}

// GetActivityStreams fetches the named data streams for an activity. Requested
// types that the activity does not have are simply omitted from the response;
// an empty result is normal and not an error.
func (c *Client) GetActivityStreams(ctx context.Context, id string, types []string) ([]ActivityStream, error) {
	values := url.Values{}
	for _, t := range types {
		values.Add("types", t)
	}
	var streams []ActivityStream
	if err := c.get(ctx, "/api/v1/activity/"+url.PathEscape(id)+"/streams", values, &streams); err != nil {
		return nil, err
	}
	return streams, nil
}

func (c *Client) GetWellness(ctx context.Context, date string) (*Wellness, error) {
	var wellness Wellness
	if err := c.get(ctx, c.athletePath("wellness", date), nil, &wellness); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &wellness, nil
}

// ListWellness fetches the daily wellness records between oldest and newest,
// both inclusive local ISO-8601 dates. Days without a record are omitted.
func (c *Client) ListWellness(ctx context.Context, oldest, newest string) ([]Wellness, error) {
	values := url.Values{}
	values.Set("oldest", oldest)
	values.Set("newest", newest)
	var records []Wellness
	if err := c.get(ctx, c.athletePath("wellness"), values, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (c *Client) GetAthleteSummary(ctx context.Context, start, end string) ([]Summary, error) {
	values := url.Values{}
	if start != "" {
		values.Set("start", start)
	}
	if end != "" {
		values.Set("end", end)
	}
	var summaries []Summary
	if err := c.get(ctx, c.athletePath("athlete-summary"), values, &summaries); err != nil {
		return nil, err
	}
	return summaries, nil
}

func (c *Client) ListEvents(ctx context.Context, oldest, newest string, categories []string, limit int) ([]Event, error) {
	values := url.Values{}
	if oldest != "" {
		values.Set("oldest", oldest)
	}
	if newest != "" {
		values.Set("newest", newest)
	}
	if len(categories) > 0 {
		values.Set("category", strings.Join(categories, ","))
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	var events []Event
	if err := c.get(ctx, c.athletePath("events"), values, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (c *Client) GetEvent(ctx context.Context, id int) (*Event, error) {
	var event Event
	if err := c.get(ctx, c.athletePath("events", strconv.Itoa(id)), nil, &event); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

// get issues the request, retrying only a short rate limit wait. The whole
// call, retries included, stays inside the configured request timeout.
func (c *Client) get(ctx context.Context, apiPath string, values url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, apiPath)
	u.RawQuery = values.Encode()
	target := u.String()

	for attempt := 1; ; attempt++ {
		err := c.doGet(ctx, target, out)

		var limited *RateLimitError
		if !errors.As(err, &limited) {
			return err
		}
		// A long Retry-After is the 15 minute or daily bucket. Waiting it out
		// would blow the request timeout, so report it and let the caller
		// decide when to come back.
		if attempt >= maxRateLimitAttempts || limited.RetryAfter > maxRateLimitSleep {
			return err
		}
		wait := limited.RetryAfter
		if wait <= 0 {
			wait = time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return err
		}
	}
}

func (c *Client) doGet(ctx context.Context, target string, out any) error {
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.release()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth("API_KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "intervals-mcp/0.1")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("intervals request failed: %w", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	c.recordRateLimit(res.Header)

	if res.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
		return &RateLimitError{
			RetryAfter: parseRetryAfter(res.Header.Get("Retry-After"), c.now()),
			Body:       strings.TrimSpace(string(body)),
		}
	}
	if res.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, res.Body)
		return ErrNotFound
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
		return &APIError{StatusCode: res.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode intervals response: %w", err)
	}
	return nil
}

// acquire blocks until a request slot is free or ctx ends.
func (c *Client) acquire(ctx context.Context) error {
	if c.sem == nil {
		return nil
	}
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("intervals request queued: %w", ctx.Err())
	}
}

func (c *Client) release() {
	if c.sem != nil {
		<-c.sem
	}
}

// recordRateLimit stores the reported budget and reports it: once when first
// seen, so the actual allowance is discoverable, and again whenever what is
// left runs low.
func (c *Client) recordRateLimit(header http.Header) {
	limit, okLimit := parseRateLimitPair(header.Get("X-RateLimit-Limit"))
	remaining, okRemaining := parseRateLimitPair(header.Get("X-RateLimit-Remaining"))
	if !okLimit || !okRemaining {
		return
	}
	observed := RateLimit{
		Observed:               true,
		FifteenMinuteLimit:     limit[0],
		FifteenMinuteRemaining: remaining[0],
		DailyLimit:             limit[1],
		DailyRemaining:         remaining[1],
	}

	c.rateMu.Lock()
	first := !c.rateLimit.Observed
	c.rateLimit = observed
	c.rateMu.Unlock()

	if c.logger == nil {
		return
	}
	attrs := []any{
		"window_remaining", observed.FifteenMinuteRemaining,
		"window_limit", observed.FifteenMinuteLimit,
		"daily_remaining", observed.DailyRemaining,
		"daily_limit", observed.DailyLimit,
	}
	if isLowBudget(observed) {
		c.logger.Warn("intervals API rate limit budget running low", attrs...)
		return
	}
	if first {
		c.logger.Info("intervals API rate limit budget", attrs...)
	}
}

func isLowBudget(limit RateLimit) bool {
	low := func(remaining, total int) bool {
		return total > 0 && float64(remaining) < float64(total)*rateLimitWarnFraction
	}
	return low(limit.FifteenMinuteRemaining, limit.FifteenMinuteLimit) ||
		low(limit.DailyRemaining, limit.DailyLimit)
}

// parseRateLimitPair reads the "<15m>,<daily>" header shape.
func parseRateLimitPair(value string) ([2]int, bool) {
	var pair [2]int
	first, second, found := strings.Cut(strings.TrimSpace(value), ",")
	if !found {
		return pair, false
	}
	window, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return pair, false
	}
	daily, err := strconv.Atoi(strings.TrimSpace(second))
	if err != nil {
		return pair, false
	}
	return [2]int{window, daily}, true
}

// parseRetryAfter accepts the documented delay in seconds and also the HTTP
// date form, which the spec permits.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if wait := when.Sub(now); wait > 0 {
			return wait
		}
	}
	return 0
}

func (c *Client) athletePath(parts ...string) string {
	all := []string{"/api/v1/athlete", url.PathEscape(c.athleteID)}
	for _, part := range parts {
		all = append(all, url.PathEscape(part))
	}
	return path.Join(all...)
}

var activityListFields = []string{
	"id",
	"name",
	"type",
	"start_date_local",
	"start_date",
	"timezone",
	"moving_time",
	"elapsed_time",
	"distance",
	"icu_distance",
	"calories",
	"carbs_used",
	"carbs_ingested",
	"icu_training_load",
	"icu_atl",
	"icu_ctl",
	"average_heartrate",
	"max_heartrate",
	"icu_intensity",
	"icu_efficiency_factor",
	"icu_power_hr",
	"decoupling",
	"average_cadence",
	"average_stride",
	"avg_lr_balance",
	"gap",
	activityFieldGCT,
	activityFieldVerticalOscillation,
	activityFieldVerticalRatio,
	activityFieldVO2MaxGarmin,
	"perceived_exertion",
	"session_rpe",
	"icu_rpe",
	"feel",
	"description",
	"interval_summary",
	"tags",
}
