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
		if !strings.Contains(r.URL.Query().Get("fields"), "icu_training_load") {
			t.Fatalf("fields missing training load: %q", r.URL.Query().Get("fields"))
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
