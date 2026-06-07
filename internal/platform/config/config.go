package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultIntervalsBaseURL = "https://intervals.icu"
	defaultMCPAddr          = ":8080"
	defaultOIDCAudience     = "authenticated"
	defaultRequestTimeout   = 15 * time.Second
	defaultShutdownTimeout  = 10 * time.Second
)

const DefaultIntervalsBaseURL = defaultIntervalsBaseURL

// ErrMissingAPIKey and ErrMissingAthleteID are returned (wrapped via
// errors.Join) by ValidateIntervals so callers can detect the first-run
// "no credentials yet" case with errors.Is instead of matching error text.
var (
	ErrMissingAPIKey    = errors.New("INTERVALS_ICU_API_KEY is required")
	ErrMissingAthleteID = errors.New("INTERVALS_ICU_ATHLETE_ID is required")
)

type SourceKind string

const (
	SourceNone     SourceKind = "none"
	SourceExplicit SourceKind = "explicit"
	SourceXDG      SourceKind = "xdg"
	SourceLocal    SourceKind = "local"
)

type Source struct {
	Kind   SourceKind
	Path   string
	Exists bool
}

type DiscoveryOptions struct {
	EnvPath     string
	EnvExplicit bool
	WorkDir     string
}

type IntervalsCredentials struct {
	APIKey    string
	AthleteID string
	BaseURL   string
}

type Config struct {
	IntervalsAPIKey    string
	IntervalsAthleteID string
	IntervalsBaseURL   string

	MCPAddr        string
	MCPPublicURL   string
	RequiredScopes []string

	SupabaseURL            string
	SupabaseAnonKey        string
	SupabaseOAuthProviders []string

	OIDCIssuerURL    string
	OIDCJWKSURL      string
	OIDCAudience     string
	OIDCAllowedEmail string
	OIDCAllowedSub   string
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
}

func LoadEffective(opts DiscoveryOptions) (Config, Source, error) {
	source, values, err := discoverValues(opts)
	if err != nil {
		return Config{}, source, err
	}
	return loadFromSources(values), source, nil
}

func LoadDiscovered(opts DiscoveryOptions) (Config, Source, error) {
	cfg, source, err := LoadEffective(opts)
	if err != nil {
		return Config{}, source, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, source, err
	}
	return cfg, source, nil
}

func LoadIntervalsDiscovered(opts DiscoveryOptions) (Config, Source, error) {
	cfg, source, err := LoadEffective(opts)
	if err != nil {
		return Config{}, source, err
	}
	if err := cfg.ValidateIntervals(); err != nil {
		return Config{}, source, err
	}
	return cfg, source, nil
}

func ConfigPath(opts DiscoveryOptions) (string, error) {
	if opts.EnvExplicit {
		if strings.TrimSpace(opts.EnvPath) == "" {
			return "", errors.New("--env requires a path")
		}
		return opts.EnvPath, nil
	}
	return xdgConfigPath()
}

func WriteIntervalsConfig(path string, creds IntervalsCredentials, overwrite bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("config path is required")
	}
	if strings.TrimSpace(creds.APIKey) == "" {
		return errors.New("INTERVALS_ICU_API_KEY is required")
	}
	if strings.TrimSpace(creds.AthleteID) == "" {
		return errors.New("INTERVALS_ICU_ATHLETE_ID is required")
	}
	baseURL := strings.TrimSpace(creds.BaseURL)
	if baseURL == "" {
		baseURL = defaultIntervalsBaseURL
	}
	if _, err := url.ParseRequestURI(trimTrailingSlash(baseURL)); err != nil {
		return fmt.Errorf("INTERVALS_ICU_BASE_URL must be a valid URL: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("config already exists at %s", path)
		}
		return fmt.Errorf("create config: %w", err)
	}
	body := strings.Join([]string{
		"# Intervals.icu credentials for intervals CLI and local MCP.",
		"INTERVALS_ICU_API_KEY=" + dotenvValue(creds.APIKey),
		"INTERVALS_ICU_ATHLETE_ID=" + dotenvValue(creds.AthleteID),
		"INTERVALS_ICU_BASE_URL=" + dotenvValue(trimTrailingSlash(baseURL)),
		"",
	}, "\n")
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return fmt.Errorf("write config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}

func IsMissingIntervalsCredentials(err error) bool {
	return errors.Is(err, ErrMissingAPIKey) || errors.Is(err, ErrMissingAthleteID)
}

func loadFromSources(values map[string]string) Config {
	lookup := func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok && cleanValue(value) != "" {
			return value, true
		}
		if values == nil {
			return "", false
		}
		value, ok := values[key]
		return value, ok
	}
	cfg := Config{
		IntervalsAPIKey:    envLookup(lookup, "INTERVALS_ICU_API_KEY", ""),
		IntervalsAthleteID: envLookup(lookup, "INTERVALS_ICU_ATHLETE_ID", ""),
		IntervalsBaseURL:   trimTrailingSlash(envLookup(lookup, "INTERVALS_ICU_BASE_URL", defaultIntervalsBaseURL)),

		MCPAddr:        envLookup(lookup, "MCP_ADDR", defaultMCPAddr),
		MCPPublicURL:   trimTrailingSlash(envLookup(lookup, "MCP_PUBLIC_URL", "")),
		RequiredScopes: csv(envLookup(lookup, "MCP_REQUIRED_SCOPE", "")),

		SupabaseURL:            trimTrailingSlash(envLookup(lookup, "SUPABASE_URL", "")),
		SupabaseAnonKey:        firstNonEmpty(envLookup(lookup, "SUPABASE_ANON_KEY", ""), envLookup(lookup, "SUPABASE_PUBLISHABLE_KEY", "")),
		SupabaseOAuthProviders: csv(envLookup(lookup, "SUPABASE_OAUTH_PROVIDERS", "")),

		OIDCIssuerURL:    trimTrailingSlash(envLookup(lookup, "OIDC_ISSUER_URL", "")),
		OIDCJWKSURL:      trimTrailingSlash(envLookup(lookup, "OIDC_JWKS_URL", "")),
		OIDCAudience:     envLookup(lookup, "OIDC_AUDIENCE", defaultOIDCAudience),
		OIDCAllowedEmail: envLookup(lookup, "OIDC_ALLOWED_EMAIL", ""),
		OIDCAllowedSub:   envLookup(lookup, "OIDC_ALLOWED_SUBJECT", ""),
		RequestTimeout:   durationLookup(lookup, "REQUEST_TIMEOUT", defaultRequestTimeout),
		ShutdownTimeout:  durationLookup(lookup, "SHUTDOWN_TIMEOUT", defaultShutdownTimeout),
	}

	if cfg.OIDCIssuerURL == "" && cfg.SupabaseURL != "" {
		cfg.OIDCIssuerURL = cfg.SupabaseURL + "/auth/v1"
	}
	if cfg.OIDCJWKSURL == "" && cfg.OIDCIssuerURL != "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuerURL + "/.well-known/jwks.json"
	}
	return cfg
}

func (c Config) Validate() error {
	var errs []error
	if err := c.ValidateIntervals(); err != nil {
		errs = append(errs, err)
	}
	required := map[string]string{
		"MCP_PUBLIC_URL": c.MCPPublicURL,
		"SUPABASE_URL":   c.SupabaseURL,
		"SUPABASE_ANON_KEY or SUPABASE_PUBLISHABLE_KEY": c.SupabaseAnonKey,
		"OIDC_ISSUER_URL": c.OIDCIssuerURL,
		"OIDC_JWKS_URL":   c.OIDCJWKSURL,
		"OIDC_AUDIENCE":   c.OIDCAudience,
	}
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
	}
	if c.OIDCAllowedEmail == "" && c.OIDCAllowedSub == "" {
		errs = append(errs, errors.New("OIDC_ALLOWED_EMAIL or OIDC_ALLOWED_SUBJECT is required"))
	}
	for key, value := range map[string]string{
		"INTERVALS_ICU_BASE_URL": c.IntervalsBaseURL,
		"MCP_PUBLIC_URL":         c.MCPPublicURL,
		"SUPABASE_URL":           c.SupabaseURL,
		"OIDC_ISSUER_URL":        c.OIDCIssuerURL,
		"OIDC_JWKS_URL":          c.OIDCJWKSURL,
	} {
		if value == "" {
			continue
		}
		if _, err := url.ParseRequestURI(value); err != nil {
			errs = append(errs, fmt.Errorf("%s must be a valid URL: %w", key, err))
		}
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, errors.New("REQUEST_TIMEOUT must be positive"))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_TIMEOUT must be positive"))
	}
	return errors.Join(errs...)
}

func (c Config) ValidateIntervals() error {
	var errs []error
	if strings.TrimSpace(c.IntervalsAPIKey) == "" {
		errs = append(errs, ErrMissingAPIKey)
	}
	if strings.TrimSpace(c.IntervalsAthleteID) == "" {
		errs = append(errs, ErrMissingAthleteID)
	}
	if c.IntervalsBaseURL != "" {
		if _, err := url.ParseRequestURI(c.IntervalsBaseURL); err != nil {
			errs = append(errs, fmt.Errorf("INTERVALS_ICU_BASE_URL must be a valid URL: %w", err))
		}
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, errors.New("REQUEST_TIMEOUT must be positive"))
	}
	return errors.Join(errs...)
}

func (c Config) MCPResource() string {
	return c.MCPPublicURL + "/mcp"
}

func (c Config) ResourceMetadataURL() string {
	return c.MCPPublicURL + "/.well-known/oauth-protected-resource"
}

func discoverValues(opts DiscoveryOptions) (Source, map[string]string, error) {
	source, err := Discover(opts)
	if err != nil {
		return source, nil, err
	}
	if !source.Exists {
		return source, nil, nil
	}
	values, err := readDotenv(source.Path, source.Kind != SourceExplicit)
	if err != nil {
		return source, nil, err
	}
	return source, values, nil
}

func Discover(opts DiscoveryOptions) (Source, error) {
	if opts.EnvExplicit {
		path := strings.TrimSpace(opts.EnvPath)
		if path == "" {
			return Source{}, errors.New("--env requires a path")
		}
		if err := requireFile(path); err != nil {
			return Source{Kind: SourceExplicit, Path: path}, err
		}
		return Source{Kind: SourceExplicit, Path: path, Exists: true}, nil
	}

	if path, err := xdgConfigPath(); err == nil {
		exists, err := fileExists(path)
		if err != nil {
			return Source{}, err
		}
		if exists {
			return Source{Kind: SourceXDG, Path: path, Exists: true}, nil
		}
	}

	workDir := opts.WorkDir
	if strings.TrimSpace(workDir) == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return Source{}, fmt.Errorf("get working directory: %w", err)
		}
	}
	localPath := filepath.Join(workDir, ".env")
	exists, err := fileExists(localPath)
	if err != nil {
		return Source{}, err
	}
	if exists {
		return Source{Kind: SourceLocal, Path: localPath, Exists: true}, nil
	}
	return Source{Kind: SourceNone}, nil
}

func readDotenv(path string, missingOK bool) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if missingOK {
				return nil, nil
			}
			return nil, fmt.Errorf("open dotenv %s: %w", path, err)
		}
		return nil, fmt.Errorf("open dotenv %s: %w", path, err)
	}
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = cleanValue(value)
		if key == "" {
			continue
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("scan dotenv: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close dotenv: %w", err)
	}
	return values, nil
}

func envLookup(lookup func(string) (string, bool), key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	value = cleanValue(value)
	if value == "" {
		return fallback
	}
	return value
}

func durationLookup(lookup func(string) (string, bool), key string, fallback time.Duration) time.Duration {
	raw := envLookup(lookup, key, "")
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func xdgConfigPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return filepath.Join(dir, "intervals-mcp", "config.env"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "intervals-mcp", "config.env"), nil
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("config file %s does not exist", path)
		}
		return fmt.Errorf("stat config file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("config path %s is a directory", path)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat config file %s: %w", path, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("config path %s is a directory", path)
	}
	return true, nil
}

func csv(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func trimTrailingSlash(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = cleanValue(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return strings.TrimSpace(unquoted)
	}
	return strings.Trim(value, `"'`)
}

func dotenvValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n#\"'") {
		return strconv.Quote(value)
	}
	return value
}
