package domain

const (
	ActivityFieldGCT                 = "GCT"
	ActivityFieldVerticalOscillation = "VerticalOscillation"
	ActivityFieldVerticalRatio       = "VerticalRatio"
	ActivityFieldVO2MaxGarmin        = "VO2MaxGarmin"
)

const (
	runningDynamicsActivityNote = "No Garmin running-dynamics activity fields or streams found. Add the community custom activity fields in Intervals.icu and re-analyze the activity."
	runningDynamicsIntervalNote = "No Garmin running-dynamics interval fields or streams found for this interval."
)

const (
	streamGarminGCT              = "GarminGCT"
	streamGarminVO               = "GarminVO"
	streamGarminVerticalRatio    = "GarminVerticalRatio"
	streamGarminStepLength       = "GarminStepLength"
	streamGarminGCTBalance       = "GarminGCTBalance"
	streamGarminGCTPercent       = "GarminGCTPercent"
	streamGarminImpactLoadFactor = "GarminImpactLoadFactor"
	streamGarminGAPPace          = "GarminGAPPace"
	streamGarminStepSpeedLoss    = "GarminStepSpeedLoss"
	streamGarminStepSpeedLossPct = "GarminStepSpeedLossPercent"
)

type RunningDynamics struct {
	Available             bool     `json:"available"`
	GroundContactTimeMs   *float64 `json:"ground_contact_time_ms,omitempty"`
	VerticalOscillationCm *float64 `json:"vertical_oscillation_cm,omitempty"`
	VerticalRatioPct      *float64 `json:"vertical_ratio_pct,omitempty"`
	StepLengthMm          *float64 `json:"step_length_mm,omitempty"`
	GCTBalancePct         *float64 `json:"gct_balance_pct,omitempty"`
	GCTPct                *float64 `json:"gct_pct,omitempty"`
	ImpactLoadFactor      *float64 `json:"impact_load_factor,omitempty"`
	GAPPaceMps            *float64 `json:"gap_pace_mps,omitempty"`
	StepSpeedLossMps      *float64 `json:"step_speed_loss_mps,omitempty"`
	StepSpeedLossPct      *float64 `json:"step_speed_loss_pct,omitempty"`
	VO2MaxGarmin          *float64 `json:"vo2_max_garmin,omitempty"`
	Note                  string   `json:"note,omitempty"`
}

type IntervalRunningDynamics struct {
	IntervalIndex   int             `json:"interval_index"`
	IntervalID      *int            `json:"interval_id,omitempty"`
	Label           string          `json:"label,omitempty"`
	Type            string          `json:"type,omitempty"`
	StartIndex      *int            `json:"start_index,omitempty"`
	EndIndex        *int            `json:"end_index,omitempty"`
	StartTime       *int            `json:"start_time,omitempty"`
	EndTime         *int            `json:"end_time,omitempty"`
	RunningDynamics RunningDynamics `json:"running_dynamics"`
}

func AggregateRunningDynamics(streams []ActivityStream) RunningDynamics {
	byType := make(map[string]*float64, len(streams))
	for _, s := range streams {
		if s.AllNull {
			continue
		}
		byType[s.Type] = meanNonNull(s.Data)
	}

	rd := RunningDynamics{
		GroundContactTimeMs:   byType[streamGarminGCT],
		VerticalOscillationCm: byType[streamGarminVO],
		VerticalRatioPct:      byType[streamGarminVerticalRatio],
		StepLengthMm:          byType[streamGarminStepLength],
		GCTBalancePct:         byType[streamGarminGCTBalance],
		GCTPct:                byType[streamGarminGCTPercent],
		ImpactLoadFactor:      byType[streamGarminImpactLoadFactor],
		GAPPaceMps:            byType[streamGarminGAPPace],
		StepSpeedLossMps:      byType[streamGarminStepSpeedLoss],
		StepSpeedLossPct:      byType[streamGarminStepSpeedLossPct],
	}
	rd.finalizeAvailability()
	return rd
}

func AggregateIntervalRunningDynamics(intervals []any, streams []ActivityStream) []IntervalRunningDynamics {
	if len(intervals) == 0 {
		return nil
	}
	out := make([]IntervalRunningDynamics, 0, len(intervals))
	for i, raw := range intervals {
		record := intervalRunningDynamicsFromRaw(i, raw, streams)
		out = append(out, record)
	}
	return out
}

func RunningDynamicsFromActivity(activity *Activity) RunningDynamics {
	var rd RunningDynamics
	if activity != nil {
		rd.GroundContactTimeMs = activity.GCT
		rd.VerticalOscillationCm = activity.VerticalOscillation
		rd.VerticalRatioPct = activity.VerticalRatio
		rd.VO2MaxGarmin = activity.VO2MaxGarmin
	}
	rd.finalizeAvailability()
	return rd
}

func MergeRunningDynamics(primary, fallback RunningDynamics) RunningDynamics {
	if primary.GroundContactTimeMs == nil {
		primary.GroundContactTimeMs = fallback.GroundContactTimeMs
	}
	if primary.VerticalOscillationCm == nil {
		primary.VerticalOscillationCm = fallback.VerticalOscillationCm
	}
	if primary.VerticalRatioPct == nil {
		primary.VerticalRatioPct = fallback.VerticalRatioPct
	}
	if primary.StepLengthMm == nil {
		primary.StepLengthMm = fallback.StepLengthMm
	}
	if primary.GCTBalancePct == nil {
		primary.GCTBalancePct = fallback.GCTBalancePct
	}
	if primary.GCTPct == nil {
		primary.GCTPct = fallback.GCTPct
	}
	if primary.ImpactLoadFactor == nil {
		primary.ImpactLoadFactor = fallback.ImpactLoadFactor
	}
	if primary.GAPPaceMps == nil {
		primary.GAPPaceMps = fallback.GAPPaceMps
	}
	if primary.StepSpeedLossMps == nil {
		primary.StepSpeedLossMps = fallback.StepSpeedLossMps
	}
	if primary.StepSpeedLossPct == nil {
		primary.StepSpeedLossPct = fallback.StepSpeedLossPct
	}
	if primary.VO2MaxGarmin == nil {
		primary.VO2MaxGarmin = fallback.VO2MaxGarmin
	}
	primary.finalizeAvailability()
	return primary
}

func intervalRunningDynamicsFromRaw(index int, raw any, streams []ActivityStream) IntervalRunningDynamics {
	interval, _ := raw.(map[string]any)
	record := IntervalRunningDynamics{
		IntervalIndex: index,
		IntervalID:    intPtrFromAny(interval["id"]),
		Label:         stringFromAny(interval["label"]),
		Type:          stringFromAny(interval["type"]),
		StartIndex:    intPtrFromAny(interval["start_index"]),
		EndIndex:      intPtrFromAny(interval["end_index"]),
		StartTime:     intPtrFromAny(interval["start_time"]),
		EndTime:       intPtrFromAny(interval["end_time"]),
	}

	dynamics := runningDynamicsFromIntervalFields(interval)
	if record.StartIndex != nil && record.EndIndex != nil {
		streamDynamics := aggregateRunningDynamicsRange(streams, *record.StartIndex, *record.EndIndex)
		dynamics = MergeRunningDynamics(dynamics, streamDynamics)
	}
	if !dynamics.Available {
		dynamics.Note = runningDynamicsIntervalNote
	}
	record.RunningDynamics = dynamics
	return record
}

func runningDynamicsFromIntervalFields(interval map[string]any) RunningDynamics {
	rd := RunningDynamics{
		GroundContactTimeMs:   floatPtrFromAny(interval[ActivityFieldGCT]),
		VerticalOscillationCm: floatPtrFromAny(interval[ActivityFieldVerticalOscillation]),
		VerticalRatioPct:      floatPtrFromAny(interval[ActivityFieldVerticalRatio]),
		VO2MaxGarmin:          floatPtrFromAny(interval[ActivityFieldVO2MaxGarmin]),
	}
	rd.finalizeAvailability()
	return rd
}

func aggregateRunningDynamicsRange(streams []ActivityStream, start, end int) RunningDynamics {
	byType := make(map[string]*float64, len(streams))
	for _, s := range streams {
		if s.AllNull {
			continue
		}
		byType[s.Type] = meanNonNullRange(s.Data, start, end)
	}

	rd := RunningDynamics{
		GroundContactTimeMs:   byType[streamGarminGCT],
		VerticalOscillationCm: byType[streamGarminVO],
		VerticalRatioPct:      byType[streamGarminVerticalRatio],
		StepLengthMm:          byType[streamGarminStepLength],
		GCTBalancePct:         byType[streamGarminGCTBalance],
		GCTPct:                byType[streamGarminGCTPercent],
		ImpactLoadFactor:      byType[streamGarminImpactLoadFactor],
		GAPPaceMps:            byType[streamGarminGAPPace],
		StepSpeedLossMps:      byType[streamGarminStepSpeedLoss],
		StepSpeedLossPct:      byType[streamGarminStepSpeedLossPct],
	}
	rd.finalizeAvailability()
	return rd
}

func meanNonNull(data []*float64) *float64 {
	var sum float64
	var n int
	for _, v := range data {
		if v != nil {
			sum += *v
			n++
		}
	}
	if n == 0 {
		return nil
	}
	mean := sum / float64(n)
	return &mean
}

func meanNonNullRange(data []*float64, start, end int) *float64 {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	if end <= start {
		return nil
	}
	return meanNonNull(data[start:end])
}

func (rd *RunningDynamics) finalizeAvailability() {
	rd.Available = rd.GroundContactTimeMs != nil || rd.VerticalOscillationCm != nil ||
		rd.VerticalRatioPct != nil || rd.StepLengthMm != nil || rd.GCTBalancePct != nil ||
		rd.GCTPct != nil || rd.ImpactLoadFactor != nil || rd.GAPPaceMps != nil ||
		rd.StepSpeedLossMps != nil || rd.StepSpeedLossPct != nil || rd.VO2MaxGarmin != nil
	if rd.Available {
		rd.Note = ""
		return
	}
	rd.Note = runningDynamicsActivityNote
}

func floatPtrFromAny(value any) *float64 {
	switch v := value.(type) {
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case int:
		f := float64(v)
		return &f
	case int64:
		f := float64(v)
		return &f
	case int32:
		f := float64(v)
		return &f
	default:
		return nil
	}
}

func intPtrFromAny(value any) *int {
	switch v := value.(type) {
	case int:
		return &v
	case int64:
		i := int(v)
		return &i
	case int32:
		i := int(v)
		return &i
	case float64:
		i := int(v)
		return &i
	case float32:
		i := int(v)
		return &i
	default:
		return nil
	}
}

func stringFromAny(value any) string {
	if v, ok := value.(string); ok {
		return v
	}
	return ""
}
