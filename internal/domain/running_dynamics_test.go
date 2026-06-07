package domain

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

func TestAggregateIntervalRunningDynamicsFromIntervalFields(t *testing.T) {
	intervals := []any{
		map[string]any{
			"id":                  float64(42),
			"label":               "Warmup",
			"type":                "WORK",
			"start_index":         float64(0),
			"end_index":           float64(10),
			"start_time":          float64(0),
			"end_time":            float64(60),
			"GCT":                 float64(278.5),
			"VerticalOscillation": float64(7.7),
			"VerticalRatio":       float64(7.91),
			"VO2MaxGarmin":        float64(43.9),
		},
	}

	got := AggregateIntervalRunningDynamics(intervals, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	record := got[0]
	if record.IntervalIndex != 0 || record.IntervalID == nil || *record.IntervalID != 42 {
		t.Fatalf("record identity = %#v", record)
	}
	if record.Label != "Warmup" || record.Type != "WORK" {
		t.Fatalf("record label/type = %#v", record)
	}
	if record.StartIndex == nil || *record.StartIndex != 0 || record.EndIndex == nil || *record.EndIndex != 10 {
		t.Fatalf("record index bounds = %#v", record)
	}
	if !record.RunningDynamics.Available {
		t.Fatalf("RunningDynamics = %#v, want available", record.RunningDynamics)
	}
	if got := record.RunningDynamics.GroundContactTimeMs; got == nil || *got != 278.5 {
		t.Fatalf("GroundContactTimeMs = %v, want 278.5", got)
	}
	if got := record.RunningDynamics.VerticalOscillationCm; got == nil || *got != 7.7 {
		t.Fatalf("VerticalOscillationCm = %v, want 7.7", got)
	}
	if got := record.RunningDynamics.VerticalRatioPct; got == nil || *got != 7.91 {
		t.Fatalf("VerticalRatioPct = %v, want 7.91", got)
	}
	if got := record.RunningDynamics.VO2MaxGarmin; got == nil || *got != 43.9 {
		t.Fatalf("VO2MaxGarmin = %v, want 43.9", got)
	}
}

func TestAggregateIntervalRunningDynamicsFromStreamRanges(t *testing.T) {
	intervals := []any{
		map[string]any{"id": float64(1), "start_index": float64(0), "end_index": float64(2)},
		map[string]any{"id": float64(2), "start_index": float64(2), "end_index": float64(4)},
	}
	streams := []ActivityStream{
		{Type: "GarminGCT", Data: []*float64{new(float64(100)), new(float64(200)), nil, new(float64(400))}},
		{Type: "GarminVO", Data: []*float64{new(float64(1)), new(float64(2)), new(float64(3)), new(float64(4))}},
	}

	got := AggregateIntervalRunningDynamics(intervals, streams)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if rd := got[0].RunningDynamics; rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 150 {
		t.Fatalf("first GroundContactTimeMs = %#v, want 150", rd.GroundContactTimeMs)
	}
	if rd := got[0].RunningDynamics; rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 1.5 {
		t.Fatalf("first VerticalOscillationCm = %#v, want 1.5", rd.VerticalOscillationCm)
	}
	if rd := got[1].RunningDynamics; rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 400 {
		t.Fatalf("second GroundContactTimeMs = %#v, want 400", rd.GroundContactTimeMs)
	}
	if rd := got[1].RunningDynamics; rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 3.5 {
		t.Fatalf("second VerticalOscillationCm = %#v, want 3.5", rd.VerticalOscillationCm)
	}
}

func TestAggregateIntervalRunningDynamicsPrefersIntervalFields(t *testing.T) {
	intervals := []any{
		map[string]any{
			"start_index": float64(0),
			"end_index":   float64(2),
			"GCT":         float64(278.5),
		},
	}
	streams := []ActivityStream{
		{Type: "GarminGCT", Data: []*float64{new(float64(100)), new(float64(200))}},
		{Type: "GarminVO", Data: []*float64{new(float64(8)), new(float64(10))}},
	}

	got := AggregateIntervalRunningDynamics(intervals, streams)
	rd := got[0].RunningDynamics
	if rd.GroundContactTimeMs == nil || *rd.GroundContactTimeMs != 278.5 {
		t.Fatalf("GroundContactTimeMs = %v, want interval field 278.5", rd.GroundContactTimeMs)
	}
	if rd.VerticalOscillationCm == nil || *rd.VerticalOscillationCm != 9 {
		t.Fatalf("VerticalOscillationCm = %v, want stream fallback 9", rd.VerticalOscillationCm)
	}
}

func TestAggregateIntervalRunningDynamicsUnavailable(t *testing.T) {
	intervals := []any{
		map[string]any{"id": float64(1), "start_index": float64(0), "end_index": float64(10)},
	}

	got := AggregateIntervalRunningDynamics(intervals, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	rd := got[0].RunningDynamics
	if rd.Available {
		t.Fatalf("Available = true, want false: %#v", rd)
	}
	if rd.Note != "No Garmin running-dynamics interval fields or streams found for this interval." {
		t.Fatalf("Note = %q", rd.Note)
	}
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
