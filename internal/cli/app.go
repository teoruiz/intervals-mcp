package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
)

type Service interface {
	TodayContext(context.Context, insights.TodayArgs) (insights.TodayContext, error)
	RecentActivities(context.Context, insights.RecentActivitiesArgs) (insights.ActivitiesContext, error)
	Activity(context.Context, insights.ActivityArgs) (*insights.ActivityDetail, error)
	Recovery(context.Context, insights.RecoveryArgs) (insights.RecoveryContext, error)
	WellnessRange(context.Context, insights.WellnessRangeArgs) (insights.WellnessRangeContext, error)
	Calendar(context.Context, insights.CalendarArgs) (insights.CalendarContext, error)
	Search(context.Context, insights.SearchArgs) (insights.SearchResult, error)
}

type GlobalOptions struct {
	EnvPath     string
	EnvExplicit bool
	JSON        bool
}

type Options struct {
	Name    string
	JSON    bool
	NoStyle bool
	In      io.Reader
	Out     io.Writer
	ErrOut  io.Writer
}

type App struct {
	service Service
	name    string
	json    bool
	style   bool
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
}

func New(service Service, opts Options) *App {
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "intervals"
	}
	return &App{
		service: service,
		name:    name,
		json:    opts.JSON,
		style:   !opts.NoStyle,
		in:      in,
		out:     out,
		errOut:  errOut,
	}
}

func ParseGlobals(args []string) (GlobalOptions, []string, error) {
	var opts GlobalOptions
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			remaining = append(remaining, args[i+1:]...)
			return opts, remaining, nil
		case arg == "--json":
			opts.JSON = true
		case arg == "--env":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return opts, nil, errors.New("--env requires a path")
			}
			opts.EnvPath = args[i]
			opts.EnvExplicit = true
		case strings.HasPrefix(arg, "--env="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--env="))
			if value == "" {
				return opts, nil, errors.New("--env requires a path")
			}
			opts.EnvPath = value
			opts.EnvExplicit = true
		default:
			remaining = append(remaining, arg)
		}
	}
	return opts, remaining, nil
}

func NeedsService(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "help", "--help", "-h":
		return false
	case "today", "activities", "activity", "recovery", "wellness", "calendar", "search", "explore":
		return !helpRequested(args[1:])
	default:
		return false
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_, err := io.WriteString(a.out, Usage(a.name))
		return err
	}

	command := args[0]
	commandArgs := args[1:]
	switch command {
	case "help", "--help", "-h":
		return a.runHelp(commandArgs)
	case "today":
		return a.runToday(ctx, commandArgs)
	case "activities":
		return a.runActivities(ctx, commandArgs)
	case "activity":
		return a.runActivity(ctx, commandArgs)
	case "recovery":
		return a.runRecovery(ctx, commandArgs)
	case "wellness":
		return a.runWellness(ctx, commandArgs)
	case "calendar":
		return a.runCalendar(ctx, commandArgs)
	case "search":
		return a.runSearch(ctx, commandArgs)
	case "explore":
		return a.runExplore(ctx, commandArgs)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, strings.TrimSpace(usageHint(a.name)))
	}
}

func (a *App) runHelp(args []string) error {
	if len(args) == 0 {
		_, err := io.WriteString(a.out, Usage(a.name))
		return err
	}
	_, err := io.WriteString(a.out, CommandUsage(a.name, args[0]))
	return err
}

func (a *App) runToday(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "today"))
		return err
	}
	fs := newFlagSet("today")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("today does not accept positional arguments")
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.TodayContext(ctx, insights.TodayArgs{})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeToday(a.out, result, a.style)
	return nil
}

func (a *App) runActivities(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "activities"))
		return err
	}
	fs := newFlagSet("activities")
	oldest := fs.String("oldest", "", "local start date in YYYY-MM-DD format")
	newest := fs.String("newest", "", "local end date in YYYY-MM-DD format")
	limit := fs.Int("limit", 0, "maximum number of activities, 1 to 50")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("activities does not accept positional arguments")
	}
	if err := validateDateFlag("oldest", *oldest); err != nil {
		return err
	}
	if err := validateDateFlag("newest", *newest); err != nil {
		return err
	}
	if *limit < 0 || *limit > 50 {
		return fmt.Errorf("limit must be between 1 and 50")
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.RecentActivities(ctx, insights.RecentActivitiesArgs{
		Oldest: *oldest,
		Newest: *newest,
		Limit:  *limit,
	})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeActivities(a.out, result.Activities, a.style)
	return nil
}

func (a *App) runActivity(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "activity"))
		return err
	}
	id, includeIntervals, includeRunningDynamics, err := parseActivityArgs(args)
	if err != nil {
		return err
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.Activity(ctx, insights.ActivityArgs{
		ID:                     id,
		IncludeIntervals:       includeIntervals,
		IncludeRunningDynamics: includeRunningDynamics,
	})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeActivity(a.out, result, a.style)
	return nil
}

func parseActivityArgs(args []string) (string, bool, bool, error) {
	var ids []string
	includeIntervals := false
	includeRunningDynamics := false
	for _, arg := range args {
		switch arg {
		case "--intervals":
			includeIntervals = true
		case "--running-dynamics":
			includeRunningDynamics = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, false, fmt.Errorf("unknown activity flag %q", arg)
			}
			ids = append(ids, arg)
		}
	}
	if len(ids) != 1 {
		return "", false, false, fmt.Errorf("activity requires exactly one activity id")
	}
	return ids[0], includeIntervals, includeRunningDynamics, nil
}

func (a *App) runRecovery(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "recovery"))
		return err
	}
	fs := newFlagSet("recovery")
	date := fs.String("date", "", "local date in YYYY-MM-DD format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("recovery does not accept positional arguments")
	}
	if err := validateDateFlag("date", *date); err != nil {
		return err
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.Recovery(ctx, insights.RecoveryArgs{Date: *date})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeRecovery(a.out, result, a.style)
	return nil
}

func (a *App) runWellness(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "wellness"))
		return err
	}
	fs := newFlagSet("wellness")
	oldest := fs.String("oldest", "", "local start date in YYYY-MM-DD format")
	newest := fs.String("newest", "", "local end date in YYYY-MM-DD format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("wellness does not accept positional arguments")
	}
	if err := validateDateFlag("oldest", *oldest); err != nil {
		return err
	}
	if err := validateDateFlag("newest", *newest); err != nil {
		return err
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.WellnessRange(ctx, insights.WellnessRangeArgs{
		Oldest: *oldest,
		Newest: *newest,
	})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeWellness(a.out, result, a.style)
	return nil
}

func (a *App) runCalendar(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "calendar"))
		return err
	}
	fs := newFlagSet("calendar")
	oldest := fs.String("oldest", "", "local start date in YYYY-MM-DD format")
	newest := fs.String("newest", "", "local end date in YYYY-MM-DD format")
	var categories categoryFlags
	fs.Var(&categories, "category", "event category; repeat or comma-separate values")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("calendar does not accept positional arguments")
	}
	if err := validateDateFlag("oldest", *oldest); err != nil {
		return err
	}
	if err := validateDateFlag("newest", *newest); err != nil {
		return err
	}
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.Calendar(ctx, insights.CalendarArgs{
		Oldest:     *oldest,
		Newest:     *newest,
		Categories: categories,
	})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeCalendar(a.out, result, a.style)
	return nil
}

func (a *App) runSearch(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "search"))
		return err
	}
	query := strings.TrimSpace(strings.Join(args, " "))
	if err := a.requireService(); err != nil {
		return err
	}
	result, err := a.service.Search(ctx, insights.SearchArgs{Query: query})
	if err != nil {
		return err
	}
	if a.json {
		return writeJSON(a.out, result)
	}
	writeSearch(a.out, result, a.style)
	return nil
}

func (a *App) runExplore(ctx context.Context, args []string) error {
	if helpRequested(args) {
		_, err := io.WriteString(a.out, CommandUsage(a.name, "explore"))
		return err
	}
	fs := newFlagSet("explore")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("explore does not accept positional arguments")
	}
	if err := a.requireService(); err != nil {
		return err
	}

	choice := "today"
	if err := a.form(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Explore Intervals data").
				Options(
					huh.NewOption("Today", "today"),
					huh.NewOption("Activities", "activities"),
					huh.NewOption("Recovery", "recovery"),
					huh.NewOption("Wellness", "wellness"),
					huh.NewOption("Calendar", "calendar"),
					huh.NewOption("Search", "search"),
				).
				Value(&choice),
		),
	).RunWithContext(ctx); err != nil {
		return err
	}

	switch choice {
	case "today":
		return a.runToday(ctx, nil)
	case "activities":
		return a.exploreActivities(ctx)
	case "recovery":
		return a.exploreRecovery(ctx)
	case "wellness":
		return a.exploreWellness(ctx)
	case "calendar":
		return a.exploreCalendar(ctx)
	case "search":
		return a.exploreSearch(ctx)
	default:
		return fmt.Errorf("unsupported explore choice %q", choice)
	}
}

func (a *App) exploreActivities(ctx context.Context) error {
	var oldest, newest string
	limit := "10"
	if err := a.form(huh.NewGroup(
		huh.NewInput().Title("Oldest date").Placeholder("YYYY-MM-DD").Value(&oldest),
		huh.NewInput().Title("Newest date").Placeholder("YYYY-MM-DD").Value(&newest),
		huh.NewInput().Title("Limit").Placeholder("10").Value(&limit),
	)).RunWithContext(ctx); err != nil {
		return err
	}
	args := []string{}
	args = appendFlag(args, "--oldest", oldest)
	args = appendFlag(args, "--newest", newest)
	args = appendFlag(args, "--limit", limit)
	return a.runActivities(ctx, args)
}

func (a *App) exploreRecovery(ctx context.Context) error {
	var date string
	if err := a.form(huh.NewGroup(
		huh.NewInput().Title("Date").Placeholder("YYYY-MM-DD").Value(&date),
	)).RunWithContext(ctx); err != nil {
		return err
	}
	args := appendFlag(nil, "--date", date)
	return a.runRecovery(ctx, args)
}

func (a *App) exploreWellness(ctx context.Context) error {
	var oldest, newest string
	if err := a.form(huh.NewGroup(
		huh.NewInput().Title("Oldest date").Placeholder("YYYY-MM-DD").Value(&oldest),
		huh.NewInput().Title("Newest date").Placeholder("YYYY-MM-DD").Value(&newest),
	)).RunWithContext(ctx); err != nil {
		return err
	}
	args := []string{}
	args = appendFlag(args, "--oldest", oldest)
	args = appendFlag(args, "--newest", newest)
	return a.runWellness(ctx, args)
}

func (a *App) exploreCalendar(ctx context.Context) error {
	var oldest, newest, categories string
	if err := a.form(huh.NewGroup(
		huh.NewInput().Title("Oldest date").Placeholder("YYYY-MM-DD").Value(&oldest),
		huh.NewInput().Title("Newest date").Placeholder("YYYY-MM-DD").Value(&newest),
		huh.NewInput().Title("Category").Placeholder("WORKOUT,NOTES").Value(&categories),
	)).RunWithContext(ctx); err != nil {
		return err
	}
	args := []string{}
	args = appendFlag(args, "--oldest", oldest)
	args = appendFlag(args, "--newest", newest)
	args = appendFlag(args, "--category", categories)
	return a.runCalendar(ctx, args)
}

func (a *App) exploreSearch(ctx context.Context) error {
	var query string
	if err := a.form(huh.NewGroup(
		huh.NewInput().Title("Search").Placeholder("ride, recovery, workout").Value(&query),
	)).RunWithContext(ctx); err != nil {
		return err
	}
	if query == "" {
		return a.runSearch(ctx, nil)
	}
	return a.runSearch(ctx, []string{query})
}

func (a *App) form(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithInput(a.in).
		WithOutput(a.errOut)
}

func (a *App) requireService() error {
	if a.service == nil {
		return errors.New("intervals service is required")
	}
	return nil
}

func appendFlag(args []string, name, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return args
	}
	return append(args, name, value)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func validateDateFlag(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return fmt.Errorf("%s must be a date in YYYY-MM-DD format", name)
	}
	return nil
}

type categoryFlags []string

func (f *categoryFlags) String() string {
	return strings.Join(*f, ",")
}

func (f *categoryFlags) Set(value string) error {
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*f = append(*f, part)
		}
	}
	return nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeLine(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeToday(w io.Writer, ctx insights.TodayContext, style bool) {
	writeLine(w, title("Today "+ctx.Date, style))
	if ctx.Timezone != "" {
		writef(w, "Timezone: %s\n", ctx.Timezone)
	}
	if ctx.Recovery == nil {
		writeLine(w, "Recovery: unavailable")
	} else {
		writef(w, "Recovery: readiness %s, HRV %s, resting HR %s, sleep %s\n",
			floatPtr(ctx.Recovery.Readiness), floatPtr(ctx.Recovery.HRV), intPtr(ctx.Recovery.RestingHR), secondsPtr(ctx.Recovery.SleepSecs))
	}
	if ctx.LastActivity == nil {
		writeLine(w, "Last activity: none")
	} else {
		writef(w, "Last activity: %s\n", activityLine(*ctx.LastActivity))
	}
	writef(w, "Nutrition: %d kcal, %dg carbs used, %dg carbs ingested, %d load\n",
		ctx.Nutrition.TotalCaloriesBurned,
		ctx.Nutrition.TotalCarbsUsedGrams,
		ctx.Nutrition.TotalCarbsIngestedGrams,
		ctx.Nutrition.TotalTrainingLoad,
	)
	if len(ctx.PlannedEvents) == 0 {
		writeLine(w, "Planned events: none")
	} else {
		writeLine(w, "Planned events:")
		writeEventTable(w, ctx.PlannedEvents)
	}
	writeNotes(w, ctx.Notes)
}

func writeActivities(w io.Writer, activities []intervals.Activity, style bool) {
	writeLine(w, title("Activities", style))
	if len(activities) == 0 {
		writeLine(w, "No activities found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	writeLine(tw, "DATE\tID\tTYPE\tNAME\tLOAD\tTIME\tDIST\tKCAL")
	for _, activity := range activities {
		writef(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			datePrefix(activity.StartDateLocal),
			activity.ID,
			activity.Type,
			fallback(activity.Name, "-"),
			intPtr(activity.TrainingLoad),
			secondsPtr(activity.MovingTime),
			distancePtr(activity.Distance),
			intPtr(activity.Calories),
		)
	}
	_ = tw.Flush()
}

func writeActivity(w io.Writer, detail *insights.ActivityDetail, style bool) {
	if detail == nil || detail.Activity == nil {
		writeLine(w, "Activity not found.")
		return
	}
	activity := detail.Activity
	writeLine(w, title(fallback(activity.Name, "Activity"), style))
	writef(w, "ID: %s\n", activity.ID)
	writef(w, "Date: %s\n", fallback(activity.StartDateLocal, "-"))
	writef(w, "Type: %s\n", fallback(activity.Type, "-"))
	writef(w, "Moving time: %s\n", secondsPtr(activity.MovingTime))
	writef(w, "Distance: %s\n", distancePtr(activity.Distance))
	writef(w, "Training load: %s\n", intPtr(activity.TrainingLoad))
	writef(w, "Calories: %s\n", intPtr(activity.Calories))
	writef(w, "Heart rate: avg %s, max %s\n", intPtr(activity.AverageHeartrate), intPtr(activity.MaxHeartrate))
	writeActivityMetrics(w, activity)
	if len(activity.IntervalSummary) > 0 {
		writef(w, "Interval summary: %s\n", strings.Join(activity.IntervalSummary, "; "))
	}
	if len(activity.Intervals) > 0 {
		writef(w, "Intervals: %d\n", len(activity.Intervals))
	}
	if len(activity.Tags) > 0 {
		writef(w, "Tags: %s\n", strings.Join(activity.Tags, ", "))
	}
	writeRunningDynamics(w, detail.RunningDynamics)
}

// writeActivityMetrics prints cadence plus run-specific summary metrics when
// present. Run cadence is normalized upstream; non-run cadence remains raw.
func writeActivityMetrics(w io.Writer, activity *intervals.Activity) {
	if activity.AverageCadence != nil {
		unit := "rpm"
		if intervals.IsRunType(activity.Type) {
			unit = "spm"
		}
		writef(w, "Cadence: %s %s\n", floatPtr(activity.AverageCadence), unit)
	}
	if activity.AverageStride != nil {
		writef(w, "Stride length: %s m\n", floatPtr(activity.AverageStride))
	}
	if activity.AvgLRBalance != nil {
		writef(w, "L/R balance: %s%%\n", floatPtr(activity.AvgLRBalance))
	}
	if activity.GAP != nil {
		writef(w, "GAP: %s\n", pacePtr(activity.GAP))
	}
}

// writeRunningDynamics prints Garmin running-dynamics averages, or the absence
// note when the streams were requested but not found.
func writeRunningDynamics(w io.Writer, rd *intervals.RunningDynamics) {
	if rd == nil {
		return
	}
	if !rd.Available {
		writef(w, "Running dynamics: %s\n", rd.Note)
		return
	}
	writeLine(w, "Running dynamics:")
	if rd.GroundContactTimeMs != nil {
		writef(w, "  Ground contact time: %s ms\n", floatPtr(rd.GroundContactTimeMs))
	}
	if rd.VerticalOscillationCm != nil {
		writef(w, "  Vertical oscillation: %s cm\n", floatPtr(rd.VerticalOscillationCm))
	}
	if rd.VerticalRatioPct != nil {
		writef(w, "  Vertical ratio: %s%%\n", floatPtr(rd.VerticalRatioPct))
	}
	if rd.StepLengthMm != nil {
		writef(w, "  Step length: %s mm\n", floatPtr(rd.StepLengthMm))
	}
	if rd.GCTBalancePct != nil {
		writef(w, "  GCT balance: %s%%\n", floatPtr(rd.GCTBalancePct))
	}
	if rd.GCTPct != nil {
		writef(w, "  GCT percent: %s%%\n", floatPtr(rd.GCTPct))
	}
	if rd.ImpactLoadFactor != nil {
		writef(w, "  Impact load factor: %s\n", floatPtr(rd.ImpactLoadFactor))
	}
	if rd.GAPPaceMps != nil {
		writef(w, "  GAP pace: %s\n", pacePtr(rd.GAPPaceMps))
	}
	if rd.StepSpeedLossMps != nil {
		writef(w, "  Step speed loss: %s m/s\n", floatPtr(rd.StepSpeedLossMps))
	}
	if rd.StepSpeedLossPct != nil {
		writef(w, "  Step speed loss: %s%%\n", floatPtr(rd.StepSpeedLossPct))
	}
	if rd.VO2MaxGarmin != nil {
		writef(w, "  Garmin VO2 max: %s\n", floatPtr(rd.VO2MaxGarmin))
	}
}

func writeRecovery(w io.Writer, ctx insights.RecoveryContext, style bool) {
	writeLine(w, title("Recovery "+ctx.Date, style))
	if ctx.Recovery == nil {
		writeLine(w, "Recovery: unavailable")
	} else {
		writef(w, "Readiness: %s\n", floatPtr(ctx.Recovery.Readiness))
		writef(w, "HRV: %s\n", floatPtr(ctx.Recovery.HRV))
		writef(w, "Resting HR: %s\n", intPtr(ctx.Recovery.RestingHR))
		writef(w, "Sleep: %s\n", secondsPtr(ctx.Recovery.SleepSecs))
		writef(w, "Sleep score: %s\n", floatPtr(ctx.Recovery.SleepScore))
		writef(w, "Kcal consumed: %s\n", intPtr(ctx.Recovery.KcalConsumed))
		if ctx.Recovery.Stress != nil {
			writef(w, "Stress: %s\n", intPtr(ctx.Recovery.Stress))
		}
		if ctx.Recovery.Steps != nil {
			writef(w, "Steps: %s\n", intPtr(ctx.Recovery.Steps))
		}
		if len(ctx.Recovery.Extra) > 0 {
			writef(w, "Extra fields: %s\n", extraFieldsLine(ctx.Recovery.Extra))
		}
	}
	if ctx.Summary != nil {
		writef(w, "Fitness: %s, fatigue: %s, form: %s, load: %s\n",
			floatPtr(ctx.Summary.Fitness),
			floatPtr(ctx.Summary.Fatigue),
			floatPtr(ctx.Summary.Form),
			intPtr(ctx.Summary.TrainingLoad),
		)
	}
	writeNotes(w, ctx.Notes)
}

func writeWellness(w io.Writer, ctx insights.WellnessRangeContext, style bool) {
	writeLine(w, title("Wellness "+ctx.Oldest+" to "+ctx.Newest, style))
	if len(ctx.Records) == 0 {
		writeLine(w, "No wellness records found.")
		writeNotes(w, ctx.Notes)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	writeLine(tw, "DATE\tREADY\tHRV\tRHR\tSLEEP\tSCORE\tSTRESS\tSTEPS\tEXTRA")
	for _, record := range ctx.Records {
		writef(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			record.ID,
			floatPtr(record.Readiness),
			floatPtr(record.HRV),
			intPtr(record.RestingHR),
			secondsPtr(record.SleepSecs),
			floatPtr(record.SleepScore),
			intPtr(record.Stress),
			intPtr(record.Steps),
			fallback(extraFieldsLine(record.Extra), "-"),
		)
	}
	_ = tw.Flush()
	writeNotes(w, ctx.Notes)
}

// extraFieldsLine flattens custom wellness fields into "key=value" pairs in
// sorted key order so table output is stable.
func extraFieldsLine(extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, extra[key]))
	}
	return strings.Join(parts, " ")
}

func writeCalendar(w io.Writer, ctx insights.CalendarContext, style bool) {
	writeLine(w, title("Calendar "+ctx.Oldest+" to "+ctx.Newest, style))
	if len(ctx.Events) == 0 {
		writeLine(w, "No events found.")
		return
	}
	writeEventTable(w, ctx.Events)
}

func writeSearch(w io.Writer, result insights.SearchResult, style bool) {
	heading := "Search results"
	if result.Query != "" {
		heading += " for " + strconv.Quote(result.Query)
	}
	writeLine(w, title(heading, style))
	if len(result.Results) == 0 {
		writeLine(w, "No records found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	writeLine(tw, "KIND\tDATE\tID\tTITLE")
	for _, record := range result.Results {
		writef(tw, "%s\t%s\t%s\t%s\n", record.Kind, record.Date, record.ID, fallback(record.Title, "-"))
	}
	_ = tw.Flush()
}

func writeEventTable(w io.Writer, events []intervals.Event) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	writeLine(tw, "DATE\tID\tCATEGORY\tTYPE\tNAME\tLOAD\tTIME")
	for _, event := range events {
		writef(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			datePrefix(event.StartDateLocal),
			eventID(event.ID),
			event.Category,
			event.Type,
			fallback(event.Name, "-"),
			intPtr(firstInt(event.TrainingLoad, event.LoadTarget)),
			secondsPtr(firstInt(event.MovingTime, event.TimeTarget)),
		)
	}
	_ = tw.Flush()
}

func writeNotes(w io.Writer, notes []string) {
	if len(notes) == 0 {
		return
	}
	writeLine(w, "Notes:")
	for _, note := range notes {
		writef(w, "- %s\n", note)
	}
}

func title(value string, style bool) string {
	if !style {
		return value
	}
	return lipgloss.NewStyle().Bold(true).Render(value)
}

func activityLine(activity intervals.Activity) string {
	parts := []string{
		fallback(activity.Name, activity.ID),
		fallback(activity.Type, "unknown type"),
		fallback(datePrefix(activity.StartDateLocal), "unknown date"),
	}
	if activity.TrainingLoad != nil {
		parts = append(parts, "load "+intPtr(activity.TrainingLoad))
	}
	if activity.MovingTime != nil {
		parts = append(parts, secondsPtr(activity.MovingTime))
	}
	return strings.Join(parts, ", ")
}

func eventID(id *int) string {
	if id == nil {
		return "-"
	}
	return strconv.Itoa(*id)
}

func firstInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func intPtr(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

func floatPtr(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'f', 1, 64)
}

func secondsPtr(value *int) string {
	if value == nil {
		return "-"
	}
	seconds := *value
	if seconds < 0 {
		return strconv.Itoa(seconds) + "s"
	}
	duration := time.Duration(seconds) * time.Second
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func distancePtr(value *float64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%.1fkm", *value/1000)
}

// pacePtr formats a speed in m/s as a pace string in min:sec per km.
func pacePtr(value *float64) string {
	if value == nil || *value <= 0 {
		return "-"
	}
	secondsPerKm := 1000 / *value
	minutes := int(secondsPerKm) / 60
	seconds := int(secondsPerKm) % 60
	return fmt.Sprintf("%d:%02d/km", minutes, seconds)
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func datePrefix(value string) string {
	if len(value) >= len(time.DateOnly) {
		return value[:len(time.DateOnly)]
	}
	return value
}

func Usage(name string) string {
	name = commandName(name)
	return fmt.Sprintf(`Usage:
  %[1]s [--env PATH] [--json] today
  %[1]s [--env PATH] [--json] activities [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD] [--limit N]
  %[1]s [--env PATH] [--json] activity <id> [--intervals] [--running-dynamics]
  %[1]s [--env PATH] [--json] recovery [--date YYYY-MM-DD]
  %[1]s [--env PATH] [--json] wellness [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD]
  %[1]s [--env PATH] [--json] calendar [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD] [--category WORKOUT]
  %[1]s [--env PATH] [--json] search [query]
  %[1]s [--env PATH] [--json] explore
  %[1]s [--env PATH] [--json] config <init|path|doctor|show>
  %[1]s [--env PATH] mcp stdio

Global flags:
  --env PATH  Load a specific dotenv file instead of discovered config.
  --json      Emit JSON for scriptable output.

Commands:
  today       Show recovery, last activity, planned events, and nutrition context.
  activities  List recent activities.
  activity    Show one activity by id.
  recovery    Show wellness and summary for a date.
  wellness    List daily wellness records for a date range.
  calendar    List planned events.
  search      Search recent activities, today's recovery, and upcoming events.
  explore     Pick a command interactively.
  config      Initialize, inspect, and validate config.
  mcp         Run local MCP transports.
`, name)
}

func CommandUsage(name, command string) string {
	name = commandName(name)
	switch command {
	case "today":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] today\n", name)
	case "activities":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] activities [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD] [--limit N]\n", name)
	case "activity":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] activity <id> [--intervals] [--running-dynamics]\n", name)
	case "recovery":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] recovery [--date YYYY-MM-DD]\n", name)
	case "wellness":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] wellness [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD]\n", name)
	case "calendar":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] calendar [--oldest YYYY-MM-DD] [--newest YYYY-MM-DD] [--category WORKOUT]\n", name)
	case "search":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] search [query]\n", name)
	case "explore":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] explore\n", name)
	case "config":
		return fmt.Sprintf("Usage: %s [--env PATH] [--json] config <init|path|doctor|show>\n", name)
	case "mcp":
		return fmt.Sprintf("Usage: %s [--env PATH] mcp stdio\n", name)
	default:
		return Usage(name)
	}
}

func usageHint(name string) string {
	return fmt.Sprintf("Run `%s help` for usage.", commandName(name))
}

func commandName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "intervals"
	}
	return name
}
