package intervalsicu

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/teoruiz/intervals-mcp/internal/domain"
)

func TestListActivitiesRequest(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/athlete/i123/activities" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("oldest"); got != "2026-06-01" {
			t.Fatalf("oldest = %q", got)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("API_KEY:secret"))
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Fatalf("Authorization = %q", got)
		}
		fields := r.URL.Query().Get("fields")
		if !strings.Contains(fields, "icu_training_load") {
			t.Fatalf("fields missing training load: %q", fields)
		}
		if !strings.Contains(fields, "average_cadence") || !strings.Contains(fields, "gap") {
			t.Fatalf("fields missing running metrics: %q", fields)
		}
		if !strings.Contains(fields, "GCT") || !strings.Contains(fields, "VerticalOscillation") {
			t.Fatalf("fields missing running dynamics activity fields: %q", fields)
		}
		return jsonResponse(200, `[{"id":"a1","name":"Ride","type":"Ride"}]`), nil
	})}

	client, err := NewClient(Config{BaseURL: "https://intervals.test", APIKey: "secret", AthleteID: "i123", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	activities, err := client.ListActivities(context.Background(), domain.ActivityQuery{Oldest: "2026-06-01", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(activities) != 1 || activities[0].ID != "a1" {
		t.Fatalf("activities = %#v", activities)
	}
}

func TestGetActivityStreamsRequest(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/activity/a1/streams" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		types := r.URL.Query()["types"]
		if len(types) == 0 {
			t.Fatalf("expected types query params, got none")
		}
		found := false
		for _, ty := range types {
			if ty == "GarminGCT" {
				found = true
			}
		}
		if !found {
			t.Fatalf("types missing GarminGCT: %v", types)
		}
		return jsonResponse(200, `[{"type":"GarminGCT","data":[200,null,210]},{"type":"GarminVO","allNull":true,"data":[null,null]}]`), nil
	})}

	client, err := NewClient(Config{BaseURL: "https://intervals.test", APIKey: "secret", AthleteID: "i123", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	streams, err := client.GetActivityRunningDynamicsStreams(context.Background(), domain.ActivityID("a1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 2 || streams[0].Type != "GarminGCT" {
		t.Fatalf("streams = %#v", streams)
	}
	if len(streams[0].Data) != 3 || streams[0].Data[1] != nil {
		t.Fatalf("data did not decode nulls: %#v", streams[0].Data)
	}
}

func TestNotFoundMapsToNilWellness(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(404, `not found`), nil
	})}

	client, err := NewClient(Config{BaseURL: "https://intervals.test", APIKey: "secret", AthleteID: "i123", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	wellness, err := client.GetRecovery(context.Background(), domain.LocalDate("2026-06-01"))
	if err != nil {
		t.Fatal(err)
	}
	if wellness != nil {
		t.Fatalf("wellness = %#v, want nil", wellness)
	}
}

func TestNormalizeActivityDoublesRunCadence(t *testing.T) {
	run := &domain.Activity{Type: "Run", AverageCadence: new(float64(75))}
	normalizeActivity(run)
	if run.AverageCadence == nil || *run.AverageCadence != 150 {
		t.Fatalf("run cadence = %v, want 150 spm", run.AverageCadence)
	}

	trail := &domain.Activity{Type: "TrailRun", AverageCadence: new(float64(80))}
	normalizeActivity(trail)
	if trail.AverageCadence == nil || *trail.AverageCadence != 160 {
		t.Fatalf("trail cadence = %v, want 160 spm", trail.AverageCadence)
	}

	ride := &domain.Activity{Type: "Ride", AverageCadence: new(float64(90))}
	normalizeActivity(ride)
	if ride.AverageCadence == nil || *ride.AverageCadence != 90 {
		t.Fatalf("ride cadence = %v, want 90 (unchanged)", ride.AverageCadence)
	}

	normalizeActivity(nil)                           // must not panic
	normalizeActivity(&domain.Activity{Type: "Run"}) // nil cadence must not panic
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
