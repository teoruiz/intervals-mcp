package intervals

import (
	"encoding/json"
	"testing"
)

func TestMeanNonNull(t *testing.T) {
	if got := meanNonNull([]*float64{nil, nil}); got != nil {
		t.Fatalf("all-null mean = %v, want nil", got)
	}
	if got := meanNonNull(nil); got != nil {
		t.Fatalf("empty mean = %v, want nil", got)
	}
	got := meanNonNull([]*float64{new(float64(200)), nil, new(float64(220))})
	if got == nil || *got != 210 {
		t.Fatalf("mean = %v, want 210", got)
	}
}

func TestAggregateRunningDynamics(t *testing.T) {
	streams := []ActivityStream{
		{Type: "GarminGCT", Data: []*float64{new(float64(200)), nil, new(float64(220))}},
		{Type: "GarminVO", Data: []*float64{new(float64(8)), new(float64(10))}},
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

func TestRunningDynamicsFromActivityFields(t *testing.T) {
	activity := &Activity{
		GCT:                 new(278.5),
		VerticalOscillation: new(7.7),
		VerticalRatio:       new(7.91),
		VO2MaxGarmin:        new(43.9),
	}

	rd := RunningDynamicsFromActivity(activity)
	if !rd.Available {
		t.Fatal("Available = false, want true")
	}
	if rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 278.5 {
		t.Fatalf("GroundContactTimeMs = %v, want 278.5", rd.GroundContactTimeMs)
	}
	if rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 7.7 {
		t.Fatalf("VerticalOscillationCm = %v, want 7.7", rd.VerticalOscillationCm)
	}
	if rd.VerticalRatioPct == nil || *rd.VerticalRatioPct != 7.91 {
		t.Fatalf("VerticalRatioPct = %v, want 7.91", rd.VerticalRatioPct)
	}
	if rd.VO2MaxGarmin == nil || *rd.VO2MaxGarmin != 43.9 {
		t.Fatalf("VO2MaxGarmin = %v, want 43.9", rd.VO2MaxGarmin)
	}
}

func TestMergeRunningDynamicsPrefersActivityFields(t *testing.T) {
	primary := RunningDynamicsFromActivity(&Activity{GCT: new(278.5)})
	fallback := AggregateRunningDynamics([]ActivityStream{
		{Type: "GarminGCT", Data: []*float64{new(float64(200))}},
		{Type: "GarminVO", Data: []*float64{new(float64(8))}},
	})

	rd := MergeRunningDynamics(primary, fallback)
	if rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 278.5 {
		t.Fatalf("GroundContactTimeMs = %v, want activity field value 278.5", rd.GroundContactTimeMs)
	}
	if rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 8 {
		t.Fatalf("VerticalOscillationCm = %v, want stream fallback 8", rd.VerticalOscillationCm)
	}
	if rd.Note != "" {
		t.Fatalf("Note = %q, want empty", rd.Note)
	}
}

func TestNormalizeActivityDoublesRunCadence(t *testing.T) {
	run := &Activity{Type: "Run", AverageCadence: new(float64(75))}
	normalizeActivity(run)
	if run.AverageCadence == nil || *run.AverageCadence != 150 {
		t.Fatalf("run cadence = %v, want 150 spm", run.AverageCadence)
	}

	trail := &Activity{Type: "TrailRun", AverageCadence: new(float64(80))}
	normalizeActivity(trail)
	if trail.AverageCadence == nil || *trail.AverageCadence != 160 {
		t.Fatalf("trail cadence = %v, want 160 spm", trail.AverageCadence)
	}

	ride := &Activity{Type: "Ride", AverageCadence: new(float64(90))}
	normalizeActivity(ride)
	if ride.AverageCadence == nil || *ride.AverageCadence != 90 {
		t.Fatalf("ride cadence = %v, want 90 (unchanged)", ride.AverageCadence)
	}

	normalizeActivity(nil)                    // must not panic
	normalizeActivity(&Activity{Type: "Run"}) // nil cadence must not panic
}

func TestActivityUnmarshalsRunningSummaryFields(t *testing.T) {
	var a Activity
	body := `{"id":"a1","average_cadence":172.5,"average_stride":1.18,"avg_lr_balance":49.6,"gap":3.5,"GCT":278.5,"VerticalOscillation":7.7,"VerticalRatio":7.91,"VO2MaxGarmin":43.9}`
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
	if a.GCT == nil || *a.GCT != 278.5 {
		t.Fatalf("GCT = %v", a.GCT)
	}
	if a.VerticalOscillation == nil || *a.VerticalOscillation != 7.7 {
		t.Fatalf("VerticalOscillation = %v", a.VerticalOscillation)
	}
	if a.VerticalRatio == nil || *a.VerticalRatio != 7.91 {
		t.Fatalf("VerticalRatio = %v", a.VerticalRatio)
	}
	if a.VO2MaxGarmin == nil || *a.VO2MaxGarmin != 43.9 {
		t.Fatalf("VO2MaxGarmin = %v", a.VO2MaxGarmin)
	}
}
