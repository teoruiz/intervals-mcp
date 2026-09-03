package intervals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	return &Client{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		athleteID:  cfg.AthleteID,
		http:       client,
		timeout:    timeout,
		athleteTTL: athleteTTL,
		now:        now,
	}, nil
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

func (c *Client) get(ctx context.Context, apiPath string, values url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, apiPath)
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
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
