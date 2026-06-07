package intervalsicu

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
	"time"

	"github.com/teoruiz/intervals-mcp/internal/domain"
)

var ErrNotFound = errors.New("intervals resource not found")

type Client struct {
	baseURL   *url.URL
	apiKey    string
	athleteID string
	http      *http.Client
	timeout   time.Duration
}

type Config struct {
	BaseURL    string
	APIKey     string
	AthleteID  string
	HTTPClient *http.Client
	Timeout    time.Duration
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
	return &Client{
		baseURL:   baseURL,
		apiKey:    cfg.APIKey,
		athleteID: cfg.AthleteID,
		http:      client,
		timeout:   timeout,
	}, nil
}

func (c *Client) AthleteID() string {
	return c.athleteID
}

func (c *Client) GetAthlete(ctx context.Context) (*domain.Athlete, error) {
	var dto athleteDTO
	if err := c.get(ctx, c.athletePath(), nil, &dto); err != nil {
		return nil, err
	}
	athlete := mapAthlete(dto)
	return &athlete, nil
}

func (c *Client) ListActivities(ctx context.Context, query domain.ActivityQuery) ([]domain.Activity, error) {
	values := url.Values{}
	values.Set("oldest", query.Oldest)
	if query.Newest != "" {
		values.Set("newest", query.Newest)
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	values.Set("fields", strings.Join(activityListFields, ","))

	var dtos []activityDTO
	if err := c.get(ctx, c.athletePath("activities"), values, &dtos); err != nil {
		return nil, err
	}
	activities := mapActivities(dtos)
	for i := range activities {
		normalizeActivity(&activities[i])
	}
	return activities, nil
}

// normalizeActivity converts raw Intervals values into the units the rest of
// the app uses. Intervals stores running cadence per leg; double it to steps
// per minute, the standard running measure shown by watches and platforms.
func normalizeActivity(a *domain.Activity) {
	if a == nil {
		return
	}
	if a.AverageCadence != nil && domain.IsRunType(a.Type) {
		spm := *a.AverageCadence * 2
		a.AverageCadence = &spm
	}
}

func (c *Client) GetActivity(ctx context.Context, id domain.ActivityID, opts domain.ActivityDetailOptions) (*domain.Activity, error) {
	values := url.Values{}
	if opts.IncludeIntervals {
		values.Set("intervals", "true")
	}
	var dto activityDTO
	if err := c.get(ctx, "/api/v1/activity/"+url.PathEscape(string(id)), values, &dto); err != nil {
		return nil, err
	}
	activity := mapActivity(dto)
	normalizeActivity(&activity)
	return &activity, nil
}

// GetActivityStreams fetches the named data streams for an activity. Requested
// types that the activity does not have are simply omitted from the response;
// an empty result is normal and not an error.
func (c *Client) getActivityStreams(ctx context.Context, id domain.ActivityID, types []string) ([]domain.ActivityStream, error) {
	values := url.Values{}
	for _, t := range types {
		values.Add("types", t)
	}
	var dtos []activityStreamDTO
	if err := c.get(ctx, "/api/v1/activity/"+url.PathEscape(string(id))+"/streams", values, &dtos); err != nil {
		return nil, err
	}
	return mapActivityStreams(dtos), nil
}

func (c *Client) GetActivityRunningDynamicsStreams(ctx context.Context, id domain.ActivityID) ([]domain.ActivityStream, error) {
	return c.getActivityStreams(ctx, id, runningDynamicsStreamTypes())
}

func (c *Client) GetRecovery(ctx context.Context, date domain.LocalDate) (*domain.Recovery, error) {
	var dto recoveryDTO
	if err := c.get(ctx, c.athletePath("wellness", string(date)), nil, &dto); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	recovery := mapRecovery(dto)
	return &recovery, nil
}

func (c *Client) GetAthleteSummary(ctx context.Context, dateRange domain.DateRange) ([]domain.AthleteSummary, error) {
	values := url.Values{}
	if dateRange.Start != "" {
		values.Set("start", dateRange.Start)
	}
	if dateRange.End != "" {
		values.Set("end", dateRange.End)
	}
	var dtos []athleteSummaryDTO
	if err := c.get(ctx, c.athletePath("athlete-summary"), values, &dtos); err != nil {
		return nil, err
	}
	return mapAthleteSummaries(dtos), nil
}

func (c *Client) ListEvents(ctx context.Context, query domain.EventQuery) ([]domain.CalendarEvent, error) {
	values := url.Values{}
	if query.Oldest != "" {
		values.Set("oldest", query.Oldest)
	}
	if query.Newest != "" {
		values.Set("newest", query.Newest)
	}
	if len(query.Categories) > 0 {
		values.Set("category", strings.Join(query.Categories, ","))
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	var dtos []calendarEventDTO
	if err := c.get(ctx, c.athletePath("events"), values, &dtos); err != nil {
		return nil, err
	}
	return mapCalendarEvents(dtos), nil
}

func (c *Client) GetEvent(ctx context.Context, id domain.EventID) (*domain.CalendarEvent, error) {
	var dto calendarEventDTO
	if err := c.get(ctx, c.athletePath("events", strconv.Itoa(int(id))), nil, &dto); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	event := mapCalendarEvent(dto)
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
	domain.ActivityFieldGCT,
	domain.ActivityFieldVerticalOscillation,
	domain.ActivityFieldVerticalRatio,
	domain.ActivityFieldVO2MaxGarmin,
	"perceived_exertion",
	"session_rpe",
	"icu_rpe",
	"feel",
	"description",
	"interval_summary",
	"tags",
}
