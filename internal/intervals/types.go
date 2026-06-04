package intervals

type Athlete struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type Activity struct {
	ID                string   `json:"id,omitempty"`
	Name              string   `json:"name,omitempty"`
	Type              string   `json:"type,omitempty"`
	StartDateLocal    string   `json:"start_date_local,omitempty"`
	StartDate         string   `json:"start_date,omitempty"`
	Timezone          string   `json:"timezone,omitempty"`
	MovingTime        *int     `json:"moving_time,omitempty"`
	ElapsedTime       *int     `json:"elapsed_time,omitempty"`
	Distance          *float64 `json:"distance,omitempty"`
	ICUDistance       *float64 `json:"icu_distance,omitempty"`
	Calories          *int     `json:"calories,omitempty"`
	CarbsUsed         *int     `json:"carbs_used,omitempty"`
	CarbsIngested     *int     `json:"carbs_ingested,omitempty"`
	TrainingLoad      *int     `json:"icu_training_load,omitempty"`
	ATL               *float64 `json:"icu_atl,omitempty"`
	CTL               *float64 `json:"icu_ctl,omitempty"`
	AverageHeartrate  *int     `json:"average_heartrate,omitempty"`
	MaxHeartrate      *int     `json:"max_heartrate,omitempty"`
	Intensity         *float64 `json:"icu_intensity,omitempty"`
	EfficiencyFactor  *float64 `json:"icu_efficiency_factor,omitempty"`
	PowerHR           *float64 `json:"icu_power_hr,omitempty"`
	Decoupling        *float64 `json:"decoupling,omitempty"`
	AverageCadence    *float64 `json:"average_cadence,omitempty"` // run cadence is normalized to steps per minute; non-run cadence is raw
	AverageStride     *float64 `json:"average_stride,omitempty"`  // meters
	AvgLRBalance      *float64 `json:"avg_lr_balance,omitempty"`  // left/right balance percent
	GAP               *float64 `json:"gap,omitempty"`             // grade-adjusted pace, m/s
	PerceivedExertion *float64 `json:"perceived_exertion,omitempty"`
	SessionRPE        *int     `json:"session_rpe,omitempty"`
	ICURPE            *int     `json:"icu_rpe,omitempty"`
	Feel              *int     `json:"feel,omitempty"`
	Description       string   `json:"description,omitempty"`
	IntervalSummary   []string `json:"interval_summary,omitempty"`
	Intervals         []any    `json:"icu_intervals,omitempty"`
	Tags              []string `json:"tags,omitempty"`
}

type Wellness struct {
	ID            string   `json:"id,omitempty"`
	CTL           *float64 `json:"ctl,omitempty"`
	ATL           *float64 `json:"atl,omitempty"`
	RampRate      *float64 `json:"rampRate,omitempty"`
	Weight        *float64 `json:"weight,omitempty"`
	RestingHR     *int     `json:"restingHR,omitempty"`
	HRV           *float64 `json:"hrv,omitempty"`
	HRVSDNN       *float64 `json:"hrvSDNN,omitempty"`
	KcalConsumed  *int     `json:"kcalConsumed,omitempty"`
	SleepSecs     *int     `json:"sleepSecs,omitempty"`
	SleepScore    *float64 `json:"sleepScore,omitempty"`
	SleepQuality  *int     `json:"sleepQuality,omitempty"`
	AvgSleepingHR *float64 `json:"avgSleepingHR,omitempty"`
	Soreness      *int     `json:"soreness,omitempty"`
	Fatigue       *int     `json:"fatigue,omitempty"`
	Stress        *int     `json:"stress,omitempty"`
	Mood          *int     `json:"mood,omitempty"`
	Motivation    *int     `json:"motivation,omitempty"`
	Hydration     *int     `json:"hydration,omitempty"`
	Readiness     *float64 `json:"readiness,omitempty"`
	Comments      string   `json:"comments,omitempty"`
	Carbohydrates *float64 `json:"carbohydrates,omitempty"`
	Protein       *float64 `json:"protein,omitempty"`
	FatTotal      *float64 `json:"fatTotal,omitempty"`
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
