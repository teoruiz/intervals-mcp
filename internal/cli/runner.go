package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/config"
	appruntime "github.com/teoruiz/intervals-mcp/internal/runtime"
)

type RunOptions struct {
	Args       []string
	BinaryName string
	In         io.Reader
	Out        io.Writer
	ErrOut     io.Writer
	WorkDir    string
}

type runner struct {
	name    string
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
	workDir string
}

func Run(ctx context.Context, opts RunOptions) error {
	r := newRunner(opts)
	global, commandArgs, err := ParseGlobals(opts.Args)
	if err != nil {
		return err
	}

	if len(commandArgs) > 0 {
		switch commandArgs[0] {
		case "config":
			return r.runConfig(ctx, global, commandArgs[1:])
		case "mcp":
			return r.runMCP(ctx, global, commandArgs[1:])
		}
	}

	var service Service
	if NeedsService(commandArgs) {
		cfg, err := r.loadIntervals(ctx, global)
		if err != nil {
			return err
		}
		service, err = appruntime.NewInsightsService(cfg, r.httpClientFor(cfg))
		if err != nil {
			return err
		}
	}

	app := New(service, Options{
		Name:   r.name,
		JSON:   global.JSON,
		In:     r.in,
		Out:    r.out,
		ErrOut: r.errOut,
	})
	return app.Run(ctx, commandArgs)
}

func newRunner(opts RunOptions) *runner {
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
	name := commandName(opts.BinaryName)
	return &runner{
		name:    name,
		in:      in,
		out:     out,
		errOut:  errOut,
		workDir: opts.WorkDir,
	}
}

func (r *runner) discoveryOptions(global GlobalOptions) config.DiscoveryOptions {
	return config.DiscoveryOptions{
		EnvPath:     global.EnvPath,
		EnvExplicit: global.EnvExplicit,
		WorkDir:     r.workDir,
	}
}

func (r *runner) loadIntervals(ctx context.Context, global GlobalOptions) (config.Config, error) {
	cfg, source, err := config.LoadIntervalsDiscovered(r.discoveryOptions(global))
	if err == nil {
		return cfg, nil
	}
	missingCreds := config.IsMissingIntervalsCredentials(err)
	if global.EnvExplicit && missingCreds {
		return config.Config{}, fmt.Errorf("%w; update %s or set INTERVALS_ICU_API_KEY and INTERVALS_ICU_ATHLETE_ID", err, global.EnvPath)
	}
	if source.Exists && missingCreds {
		return config.Config{}, fmt.Errorf("%w; update %s or run `%s config init --force`", err, source.Path, r.name)
	}
	if global.EnvExplicit || source.Exists || !missingCreds {
		return config.Config{}, err
	}
	if !r.interactive() {
		return config.Config{}, fmt.Errorf("%w; run `%s config init` or set INTERVALS_ICU_API_KEY and INTERVALS_ICU_ATHLETE_ID", err, r.name)
	}
	if err := r.runConfigInit(ctx, global, nil, true); err != nil {
		return config.Config{}, err
	}
	cfg, _, err = config.LoadIntervalsDiscovered(r.discoveryOptions(global))
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func (r *runner) interactive() bool {
	return isTerminal(r.in) && isTerminal(r.out)
}

func isTerminal(value any) bool {
	file, ok := value.(*os.File)
	return ok && term.IsTerminal(file.Fd())
}

func (r *runner) httpClientFor(cfg config.Config) *http.Client {
	return &http.Client{Timeout: cfg.RequestTimeout}
}

func (r *runner) runMCP(ctx context.Context, global GlobalOptions, args []string) error {
	if len(args) == 0 || helpRequested(args) {
		_, err := io.WriteString(r.out, CommandUsage(r.name, "mcp"))
		return err
	}
	switch args[0] {
	case "stdio":
		fs := newFlagSet("mcp stdio")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("mcp stdio does not accept positional arguments")
		}
		cfg, err := r.loadIntervals(ctx, global)
		if err != nil {
			return err
		}
		server, err := appruntime.NewMCPServer(cfg, r.httpClientFor(cfg))
		if err != nil {
			return err
		}
		err = server.Run(ctx, &mcp.StdioTransport{})
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unknown mcp command %q\n\n%s", args[0], strings.TrimSpace(CommandUsage(r.name, "mcp")))
	}
}

func (r *runner) runConfig(ctx context.Context, global GlobalOptions, args []string) error {
	if len(args) == 0 || helpRequested(args) {
		_, err := io.WriteString(r.out, CommandUsage(r.name, "config"))
		return err
	}
	switch args[0] {
	case "init":
		return r.runConfigInit(ctx, global, args[1:], false)
	case "path":
		return r.runConfigPath(global, args[1:])
	case "show":
		return r.runConfigShow(global, args[1:])
	case "doctor":
		return r.runConfigDoctor(ctx, global, args[1:])
	default:
		return fmt.Errorf("unknown config command %q\n\n%s", args[0], strings.TrimSpace(CommandUsage(r.name, "config")))
	}
}

func (r *runner) runConfigInit(ctx context.Context, global GlobalOptions, args []string, firstRun bool) error {
	force := false
	if !firstRun {
		fs := newFlagSet("config init")
		fs.BoolVar(&force, "force", false, "overwrite existing config")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("config init does not accept positional arguments")
		}
	}
	if !r.interactive() {
		return fmt.Errorf("config init requires an interactive terminal; set INTERVALS_ICU_API_KEY and INTERVALS_ICU_ATHLETE_ID or create %s manually", configPathHint(global, r.workDir))
	}
	path, err := config.ConfigPath(r.discoveryOptions(global))
	if err != nil {
		return err
	}

	creds := config.IntervalsCredentials{BaseURL: config.DefaultIntervalsBaseURL}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Intervals.icu API key").
			EchoMode(huh.EchoModePassword).
			Value(&creds.APIKey).
			Validate(required("INTERVALS_ICU_API_KEY")),
		huh.NewInput().
			Title("Intervals.icu athlete id").
			Value(&creds.AthleteID).
			Validate(required("INTERVALS_ICU_ATHLETE_ID")),
		huh.NewInput().
			Title("Intervals.icu base URL").
			Value(&creds.BaseURL).
			Validate(validOptionalURL("INTERVALS_ICU_BASE_URL")),
	)).
		WithInput(r.in).
		WithOutput(r.errOut)
	if err := form.RunWithContext(ctx); err != nil {
		return err
	}
	if err := config.WriteIntervalsConfig(path, creds, force); err != nil {
		return err
	}
	w := r.out
	if firstRun {
		w = r.errOut
	}
	writef(w, "Wrote config to %s\n", path)
	return nil
}

func required(name string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
}

func validOptionalURL(name string) func(string) error {
	return func(value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if _, err := url.ParseRequestURI(value); err != nil {
			return fmt.Errorf("%s must be a valid URL: %w", name, err)
		}
		return nil
	}
}

func configPathHint(global GlobalOptions, workDir string) string {
	path, err := config.ConfigPath(config.DiscoveryOptions{
		EnvPath:     global.EnvPath,
		EnvExplicit: global.EnvExplicit,
		WorkDir:     workDir,
	})
	if err != nil {
		return "an intervals-mcp config.env file"
	}
	return path
}

func parseNoArgs(name string, args []string) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s does not accept positional arguments", name)
	}
	return nil
}
