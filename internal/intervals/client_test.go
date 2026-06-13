package intervals

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
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
	activities, err := client.ListActivities(context.Background(), "2026-06-01", "", 10)
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
	streams, err := client.GetActivityStreams(context.Background(), "a1", RunningDynamicsStreamTypes())
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

func TestListWellnessRequest(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/athlete/i123/wellness" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("oldest"); got != "2026-06-01" {
			t.Fatalf("oldest = %q", got)
		}
		if got := r.URL.Query().Get("newest"); got != "2026-06-07" {
			t.Fatalf("newest = %q", got)
		}
		return jsonResponse(200, `[
			{"id":"2026-06-01","stress":2,"hrv":68.0,"restingHR":47,"steps":9904,"updated":"2026-06-01T22:00:00Z","sportInfo":[],"BodyBatteryMax":82,"AvgStress":31,"vo2max":null},
			{"id":"2026-06-02","sleepScore":74.0}
		]`), nil
	})}

	client, err := NewClient(Config{BaseURL: "https://intervals.test", APIKey: "secret", AthleteID: "i123", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	records, err := client.ListWellness(context.Background(), "2026-06-01", "2026-06-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != "2026-06-01" {
		t.Fatalf("records = %#v", records)
	}
	first := records[0]
	if first.Stress == nil || *first.Stress != 2 {
		t.Fatalf("Stress = %v, want 2", first.Stress)
	}
	if first.Steps == nil || *first.Steps != 9904 {
		t.Fatalf("Steps = %v, want 9904", first.Steps)
	}
	if got := first.Extra["BodyBatteryMax"]; got != float64(82) {
		t.Fatalf("Extra[BodyBatteryMax] = %v, want 82", got)
	}
	if got := first.Extra["AvgStress"]; got != float64(31) {
		t.Fatalf("Extra[AvgStress] = %v, want 31", got)
	}
	for _, noise := range []string{"updated", "sportInfo", "vo2max"} {
		if _, ok := first.Extra[noise]; ok {
			t.Fatalf("Extra unexpectedly contains %q: %#v", noise, first.Extra)
		}
	}
	if records[1].Extra != nil {
		t.Fatalf("Extra = %#v, want nil when no custom fields", records[1].Extra)
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
	wellness, err := client.GetWellness(context.Background(), "2026-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if wellness != nil {
		t.Fatalf("wellness = %#v, want nil", wellness)
	}
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
