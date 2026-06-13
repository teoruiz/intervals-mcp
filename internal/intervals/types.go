package intervals

import (
	"encoding/json"
	"reflect"
	"strings"
)

type Athlete struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type Activity struct {
	ID                  string   `json:"id,omitempty"`
	Name                string   `json:"name,omitempty"`
	Type                string   `json:"type,omitempty"`
	StartDateLocal      string   `json:"start_date_local,omitempty"`
	StartDate           string   `json:"start_date,omitempty"`
	Timezone            string   `json:"timezone,omitempty"`
	MovingTime          *int     `json:"moving_time,omitempty"`
	ElapsedTime         *int     `json:"elapsed_time,omitempty"`
	Distance            *float64 `json:"distance,omitempty"`
	ICUDistance         *float64 `json:"icu_distance,omitempty"`
	Calories            *int     `json:"calories,omitempty"`
	CarbsUsed           *int     `json:"carbs_used,omitempty"`
	CarbsIngested       *int     `json:"carbs_ingested,omitempty"`
	TrainingLoad        *int     `json:"icu_training_load,omitempty"`
	ATL                 *float64 `json:"icu_atl,omitempty"`
	CTL                 *float64 `json:"icu_ctl,omitempty"`
	AverageHeartrate    *int     `json:"average_heartrate,omitempty"`
	MaxHeartrate        *int     `json:"max_heartrate,omitempty"`
	Intensity           *float64 `json:"icu_intensity,omitempty"`
	EfficiencyFactor    *float64 `json:"icu_efficiency_factor,omitempty"`
	PowerHR             *float64 `json:"icu_power_hr,omitempty"`
	Decoupling          *float64 `json:"decoupling,omitempty"`
	AverageCadence      *float64 `json:"average_cadence,omitempty"`     // run cadence is normalized to steps per minute; non-run cadence is raw
	AverageStride       *float64 `json:"average_stride,omitempty"`      // meters
	AvgLRBalance        *float64 `json:"avg_lr_balance,omitempty"`      // left/right balance percent
	GAP                 *float64 `json:"gap,omitempty"`                 // grade-adjusted pace, m/s
	GCT                 *float64 `json:"GCT,omitempty"`                 // Garmin ground contact time, ms
	VerticalOscillation *float64 `json:"VerticalOscillation,omitempty"` // Garmin vertical oscillation, cm
	VerticalRatio       *float64 `json:"VerticalRatio,omitempty"`       // Garmin vertical ratio, percent
	VO2MaxGarmin        *float64 `json:"VO2MaxGarmin,omitempty"`        // Garmin VO2 max estimate
	PerceivedExertion   *float64 `json:"perceived_exertion,omitempty"`
	SessionRPE          *int     `json:"session_rpe,omitempty"`
	ICURPE              *int     `json:"icu_rpe,omitempty"`
	Feel                *int     `json:"feel,omitempty"`
	Description         string   `json:"description,omitempty"`
	IntervalSummary     []string `json:"interval_summary,omitempty"`
	Intervals           []any    `json:"icu_intervals,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

type Wellness struct {
	ID              string   `json:"id,omitempty"`
	CTL             *float64 `json:"ctl,omitempty"`
	ATL             *float64 `json:"atl,omitempty"`
	RampRate        *float64 `json:"rampRate,omitempty"`
	Weight          *float64 `json:"weight,omitempty"`
	RestingHR       *int     `json:"restingHR,omitempty"`
	HRV             *float64 `json:"hrv,omitempty"`
	HRVSDNN         *float64 `json:"hrvSDNN,omitempty"`
	BaevskySI       *float64 `json:"baevskySI,omitempty"`
	KcalConsumed    *int     `json:"kcalConsumed,omitempty"`
	SleepSecs       *int     `json:"sleepSecs,omitempty"`
	SleepScore      *float64 `json:"sleepScore,omitempty"`
	SleepQuality    *int     `json:"sleepQuality,omitempty"`
	AvgSleepingHR   *float64 `json:"avgSleepingHR,omitempty"`
	Soreness        *int     `json:"soreness,omitempty"`
	Fatigue         *int     `json:"fatigue,omitempty"`
	Stress          *int     `json:"stress,omitempty"`
	Mood            *int     `json:"mood,omitempty"`
	Motivation      *int     `json:"motivation,omitempty"`
	Injury          *int     `json:"injury,omitempty"`
	Hydration       *int     `json:"hydration,omitempty"`
	HydrationVolume *float64 `json:"hydrationVolume,omitempty"`
	Readiness       *float64 `json:"readiness,omitempty"`
	Steps           *int     `json:"steps,omitempty"`
	Respiration     *float64 `json:"respiration,omitempty"`
	SpO2            *float64 `json:"spO2,omitempty"`
	VO2Max          *float64 `json:"vo2max,omitempty"`
	Systolic        *int     `json:"systolic,omitempty"`
	Diastolic       *int     `json:"diastolic,omitempty"`
	BloodGlucose    *float64 `json:"bloodGlucose,omitempty"`
	Lactate         *float64 `json:"lactate,omitempty"`
	BodyFat         *float64 `json:"bodyFat,omitempty"`
	MenstrualPhase  string   `json:"menstrualPhase,omitempty"`
	Comments        string   `json:"comments,omitempty"`
	Carbohydrates   *float64 `json:"carbohydrates,omitempty"`
	Protein         *float64 `json:"protein,omitempty"`
	FatTotal        *float64 `json:"fatTotal,omitempty"`
	// Extra carries wellness keys not modeled above, mainly custom wellness
	// fields such as Garmin stress or Body Battery synced by Intervals.icu.
	Extra map[string]any `json:"extra_fields,omitempty"`
}

// wellnessNoiseKeys are wellness response keys that carry no athlete metric
// value and would otherwise clutter Extra.
var wellnessNoiseKeys = map[string]bool{
	"updated":                 true,
	"locked":                  true,
	"tempWeight":              true,
	"tempRestingHR":           true,
	"sportInfo":               true,
	"atlLoad":                 true,
	"ctlLoad":                 true,
	"menstrualPhasePredicted": true,
}

var wellnessKnownKeys = jsonFieldNames(Wellness{})

// UnmarshalJSON decodes the modeled wellness fields and keeps any remaining
// non-null keys (custom wellness fields, future built-ins) in Extra.
func (w *Wellness) UnmarshalJSON(data []byte) error {
	type wellnessAlias Wellness
	var alias wellnessAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		if wellnessKnownKeys[key] || wellnessNoiseKeys[key] || string(value) == "null" {
			continue
		}
		var decoded any
		if err := json.Unmarshal(value, &decoded); err != nil {
			continue
		}
		if alias.Extra == nil {
			alias.Extra = map[string]any{}
		}
		alias.Extra[key] = decoded
	}
	*w = Wellness(alias)
	return nil
}

// jsonFieldNames collects the JSON keys of a struct's tagged fields.
func jsonFieldNames(value any) map[string]bool {
	names := map[string]bool{}
	t := reflect.TypeOf(value)
	for field := range t.Fields() {
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			names[name] = true
		}
	}
	return names
}

type Summary struct {
	Date         string            `json:"date,omitempty"`
	Fitness      *float64          `json:"fitness,omitempty"`
	Fatigue      *float64          `json:"fatigue,omitempty"`
	Form         *float64          `json:"form,omitempty"`
	RampRate     *float64          `json:"rampRate,omitempty"`
	TrainingLoad *int              `json:"training_load,omitempty"`
	Calories     *int              `json:"calories,omitempty"`
	Distance     *float64          `json:"distance,omitempty"`
	MovingTime   *int              `json:"moving_time,omitempty"`
	ElapsedTime  *int              `json:"elapsed_time,omitempty"`
	Weight       *float64          `json:"weight,omitempty"`
	EFTP         *float64          `json:"eftp,omitempty"`
	EFTPPerKg    *float64          `json:"eftpPerKg,omitempty"`
	TimeInZones  []int             `json:"timeInZones,omitempty"`
	ByCategory   []CategorySummary `json:"byCategory,omitempty"`
}

type CategorySummary struct {
	Category           string   `json:"category,omitempty"`
	Count              *int     `json:"count,omitempty"`
	Time               *int     `json:"time,omitempty"`
	MovingTime         *int     `json:"moving_time,omitempty"`
	ElapsedTime        *int     `json:"elapsed_time,omitempty"`
	Calories           *int     `json:"calories,omitempty"`
	TotalElevationGain *float64 `json:"total_elevation_gain,omitempty"`
	TrainingLoad       *int     `json:"training_load,omitempty"`
	Distance           *float64 `json:"distance,omitempty"`
}

type Event struct {
	ID             *int     `json:"id,omitempty"`
	Name           string   `json:"name,omitempty"`
	Description    string   `json:"description,omitempty"`
	Category       string   `json:"category,omitempty"`
	Type           string   `json:"type,omitempty"`
	StartDateLocal string   `json:"start_date_local,omitempty"`
	EndDateLocal   string   `json:"end_date_local,omitempty"`
	MovingTime     *int     `json:"moving_time,omitempty"`
	TimeTarget     *int     `json:"time_target,omitempty"`
	Distance       *float64 `json:"distance,omitempty"`
	DistanceTarget *float64 `json:"distance_target,omitempty"`
	LoadTarget     *int     `json:"load_target,omitempty"`
	TrainingLoad   *int     `json:"icu_training_load,omitempty"`
	Intensity      *float64 `json:"icu_intensity,omitempty"`
	CarbsPerHour   *int     `json:"carbs_per_hour,omitempty"`
	CarbsUsed      *int     `json:"carbs_used,omitempty"`
	Indoor         *bool    `json:"indoor,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}
