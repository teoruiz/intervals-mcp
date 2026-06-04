package intervals

import (
	"encoding/json"
	"testing"
)

//go:fix inline
func f64(v float64) *float64 { return new(v) }

func TestMeanNonNull(t *testing.T) {
	if got := meanNonNull([]*float64{nil, nil}); got != nil {
		t.Fatalf("all-null mean = %v, want nil", got)
	}
	if got := meanNonNull(nil); got != nil {
		t.Fatalf("empty mean = %v, want nil", got)
	}
	got := meanNonNull([]*float64{f64(200), nil, f64(220)})
	if got == nil || *got != 210 {
		t.Fatalf("mean = %v, want 210", got)
	}
}

func TestAggregateRunningDynamics(t *testing.T) {
	streams := []ActivityStream{
		{Type: "GarminGCT", Data: []*float64{f64(200), nil, f64(220)}},
		{Type: "GarminVO", Data: []*float64{f64(8), f64(10)}},
		{Type: "GarminGCTBalance", AllNull: true, Data: []*float64{nil, nil}},
	}
	rd := AggregateRunningDynamics(streams)
	if !rd.Available {
		t.Fatal("Available = false, want true")
	}
	if rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 210 {
		t.Fatalf("GroundContactTimeMs = %v, want 210", rd.GroundContactTimeMs)
	}
	if rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 9 {
		t.Fatalf("VerticalOscillationCm = %v, want 9", rd.VerticalOscillationCm)
	}
	if rd.GCTBalancePct != nil {
		t.Fatalf("GCTBalancePct = %v, want nil (all-null stream skipped)", rd.GCTBalancePct)
	}
	if rd.Note != "" {
		t.Fatalf("Note = %q, want empty", rd.Note)
	}
}

func TestAggregateRunningDynamicsAbsent(t *testing.T) {
	rd := AggregateRunningDynamics(nil)
	if rd.Available {
		t.Fatal("Available = true, want false")
	}
	if rd.Note == "" {
		t.Fatal("expected absence note")
	}
}

func TestNormalizeActivityDoublesRunCadence(t *testing.T) {
	run := &Activity{Type: "Run", AverageCadence: f64(75)}
	normalizeActivity(run)
	if run.AverageCadence == nil || *run.AverageCadence != 150 {
		t.Fatalf("run cadence = %v, want 150 spm", run.AverageCadence)
	}

	trail := &Activity{Type: "TrailRun", AverageCadence: f64(80)}
	normalizeActivity(trail)
	if trail.AverageCadence == nil || *trail.AverageCadence != 160 {
		t.Fatalf("trail cadence = %v, want 160 spm", trail.AverageCadence)
	}

	ride := &Activity{Type: "Ride", AverageCadence: f64(90)}
	normalizeActivity(ride)
	if ride.AverageCadence == nil || *ride.AverageCadence != 90 {
		t.Fatalf("ride cadence = %v, want 90 (unchanged)", ride.AverageCadence)
	}

	normalizeActivity(nil)                    // must not panic
	normalizeActivity(&Activity{Type: "Run"}) // nil cadence must not panic
}

func TestActivityUnmarshalsRunningSummaryFields(t *testing.T) {
	var a Activity
	body := `{"id":"a1","average_cadence":172.5,"average_stride":1.18,"avg_lr_balance":49.6,"gap":3.5}`
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatal(err)
	}
	if a.AverageCadence == nil || *a.AverageCadence != 172.5 {
		t.Fatalf("AverageCadence = %v", a.AverageCadence)
	}
	if a.AverageStride == nil || *a.AverageStride != 1.18 {
		t.Fatalf("AverageStride = %v", a.AverageStride)
	}
	if a.AvgLRBalance == nil || *a.AvgLRBalance != 49.6 {
		t.Fatalf("AvgLRBalance = %v", a.AvgLRBalance)
	}
	if a.GAP == nil || *a.GAP != 3.5 {
		t.Fatalf("GAP = %v", a.GAP)
	}
}
