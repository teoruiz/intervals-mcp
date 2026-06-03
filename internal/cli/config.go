package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/teoruiz/intervals-mcp/internal/config"
	appruntime "github.com/teoruiz/intervals-mcp/internal/runtime"
)

type sourceView struct {
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Explicit bool   `json:"explicit"`
	Exists   bool   `json:"exists"`
}

type configValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type configShow struct {
	Source sourceView    `json:"source"`
	Values []configValue `json:"values"`
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type doctorResult struct {
	Source sourceView    `json:"source"`
	Path   string        `json:"path,omitempty"`
	Checks []doctorCheck `json:"checks"`
}

func (r *runner) runConfigPath(global GlobalOptions, args []string) error {
	if err := parseNoArgs("config path", args); err != nil {
		return err
	}
	path, err := config.ConfigPath(r.discoveryOptions(global))
	if err != nil {
		return err
	}
	if global.JSON {
		return writeJSON(r.out, map[string]string{"path": path})
	}
	writeLine(r.out, path)
	return nil
}

func (r *runner) runConfigShow(global GlobalOptions, args []string) error {
	if err := parseNoArgs("config show", args); err != nil {
		return err
	}
	cfg, source, err := config.LoadEffective(r.discoveryOptions(global))
	if err != nil {
		return err
	}
	show := configShow{
		Source: sourceToView(source),
		Values: redactedConfigValues(cfg),
	}
	if global.JSON {
		return writeJSON(r.out, show)
	}
	writeSource(r.out, source)
	for _, value := range show.Values {
		writef(r.out, "%s=%s\n", value.Key, value.Value)
	}
	return nil
}

func (r *runner) runConfigDoctor(ctx context.Context, global GlobalOptions, args []string) error {
	if err := parseNoArgs("config doctor", args); err != nil {
		return err
	}
	cfg, source, err := config.LoadEffective(r.discoveryOptions(global))
	if err != nil {
		return err
	}
	// Report the discovered file's own path; only fall back to the canonical
	// XDG path (where `config init` would write) when nothing was discovered,
	// so a repo-local .env doesn't print two divergent paths.
	path := source.Path
	if source.Kind == config.SourceNone {
		path, _ = config.ConfigPath(r.discoveryOptions(global))
	}
	result := doctorResult{
		Source: sourceToView(source),
		Path:   path,
		Checks: doctorChecks(ctx, cfg, source, r),
	}
	if global.JSON {
		return writeJSON(r.out, result)
	}
	writeSource(r.out, source)
	if path != "" {
		writef(r.out, "Config path: %s\n", path)
	}
	for _, check := range result.Checks {
		if check.Detail == "" {
			writef(r.out, "%s: %s\n", check.Name, check.Status)
			continue
		}
		writef(r.out, "%s: %s (%s)\n", check.Name, check.Status, check.Detail)
	}
	return nil
}

func doctorChecks(ctx context.Context, cfg config.Config, source config.Source, r *runner) []doctorCheck {
	checks := []doctorCheck{}
	if source.Exists {
		checks = append(checks, doctorCheck{Name: "config_source", Status: "ok", Detail: string(source.Kind)})
		checks = append(checks, fileModeCheck(source.Path))
	} else {
		checks = append(checks, doctorCheck{Name: "config_source", Status: "warn", Detail: "no config file discovered"})
	}

	if strings.TrimSpace(cfg.IntervalsAPIKey) == "" {
		checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_API_KEY", Status: "missing"})
	} else {
		checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_API_KEY", Status: "ok", Detail: "set"})
	}
	if strings.TrimSpace(cfg.IntervalsAthleteID) == "" {
		checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_ATHLETE_ID", Status: "missing"})
	} else {
		checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_ATHLETE_ID", Status: "ok", Detail: "set"})
	}
	if _, err := url.ParseRequestURI(cfg.IntervalsBaseURL); err != nil {
		checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_BASE_URL", Status: "invalid", Detail: err.Error()})
		return checks
	}
	checks = append(checks, doctorCheck{Name: "INTERVALS_ICU_BASE_URL", Status: "ok", Detail: cfg.IntervalsBaseURL})

	if err := cfg.ValidateIntervals(); err != nil {
		checks = append(checks, doctorCheck{Name: "intervals_config", Status: "invalid", Detail: err.Error()})
		return checks
	}
	client, err := appruntime.NewIntervalsClient(cfg, r.httpClientFor(cfg))
	if err != nil {
		checks = append(checks, doctorCheck{Name: "intervals_client", Status: "invalid", Detail: err.Error()})
		return checks
	}
	athlete, err := client.GetAthlete(ctx)
	if err != nil {
		checks = append(checks, doctorCheck{Name: "intervals_read", Status: "fail", Detail: err.Error()})
		return checks
	}
	detail := "athlete reachable"
	if athlete != nil && athlete.ID != "" {
		detail = "athlete " + athlete.ID + " reachable"
	}
	checks = append(checks, doctorCheck{Name: "intervals_read", Status: "ok", Detail: detail})
	return checks
}

func fileModeCheck(path string) doctorCheck {
	info, err := os.Stat(path)
	if err != nil {
		return doctorCheck{Name: "file_permissions", Status: "warn", Detail: err.Error()}
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return doctorCheck{Name: "file_permissions", Status: "warn", Detail: fmt.Sprintf("mode %04o should be 0600", mode)}
	}
	return doctorCheck{Name: "file_permissions", Status: "ok", Detail: fmt.Sprintf("mode %04o", mode)}
}

func sourceToView(source config.Source) sourceView {
	return sourceView{
		Kind:     string(source.Kind),
		Path:     source.Path,
		Explicit: source.Kind == config.SourceExplicit,
		Exists:   source.Exists,
	}
}

func writeSource(w io.Writer, source config.Source) {
	if source.Exists {
		writef(w, "Config source: %s (%s)\n", source.Path, source.Kind)
		return
	}
	writeLine(w, "Config source: none")
}

func redactedConfigValues(cfg config.Config) []configValue {
	values := []configValue{
		{"INTERVALS_ICU_API_KEY", redact("INTERVALS_ICU_API_KEY", cfg.IntervalsAPIKey)},
		{"INTERVALS_ICU_ATHLETE_ID", cfg.IntervalsAthleteID},
		{"INTERVALS_ICU_BASE_URL", cfg.IntervalsBaseURL},
		{"MCP_ADDR", cfg.MCPAddr},
		{"MCP_PUBLIC_URL", cfg.MCPPublicURL},
		{"MCP_REQUIRED_SCOPE", strings.Join(cfg.RequiredScopes, ",")},
		{"SUPABASE_URL", cfg.SupabaseURL},
		{"SUPABASE_ANON_KEY", redact("SUPABASE_ANON_KEY", cfg.SupabaseAnonKey)},
		{"SUPABASE_OAUTH_PROVIDERS", strings.Join(cfg.SupabaseOAuthProviders, ",")},
		{"OIDC_ISSUER_URL", cfg.OIDCIssuerURL},
		{"OIDC_JWKS_URL", cfg.OIDCJWKSURL},
		{"OIDC_AUDIENCE", cfg.OIDCAudience},
		{"OIDC_ALLOWED_EMAIL", cfg.OIDCAllowedEmail},
		{"OIDC_ALLOWED_SUBJECT", cfg.OIDCAllowedSub},
		{"REQUEST_TIMEOUT", cfg.RequestTimeout.String()},
		{"SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout.String()},
	}
	return values
}

func redact(key, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN") || strings.Contains(key, "SECRET") {
		return "REDACTED"
	}
	return value
}
