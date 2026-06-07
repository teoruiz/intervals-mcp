package intervalsicu

// Garmin running-dynamics custom stream type names used by Intervals.icu. These
// are adapter details; application use cases ask for running-dynamics streams
// without knowing the provider-specific names.
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

func runningDynamicsStreamTypes() []string {
	return []string{
		streamGarminGCT,
		streamGarminVO,
		streamGarminVerticalRatio,
		streamGarminStepLength,
		streamGarminGCTBalance,
		streamGarminGCTPercent,
		streamGarminImpactLoadFactor,
		streamGarminGAPPace,
		streamGarminStepSpeedLoss,
		streamGarminStepSpeedLossPct,
	}
}
