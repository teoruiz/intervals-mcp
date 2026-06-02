package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultIntervalsBaseURL = "https://intervals.icu"
	defaultMCPAddr          = ":8080"
	defaultOIDCAudience     = "authenticated"
	defaultRequestTimeout   = 15 * time.Second
)

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

func Load(dotenvPath string) (Config, error) {
	if dotenvPath != "" {
		if err := loadDotenv(dotenvPath); err != nil {
			return Config{}, err
		}
	}

	cfg := Config{
		IntervalsAPIKey:    env("INTERVALS_ICU_API_KEY", ""),
		IntervalsAthleteID: env("INTERVALS_ICU_ATHLETE_ID", ""),
		IntervalsBaseURL:   trimTrailingSlash(env("INTERVALS_ICU_BASE_URL", defaultIntervalsBaseURL)),

		MCPAddr:        env("MCP_ADDR", defaultMCPAddr),
		MCPPublicURL:   trimTrailingSlash(env("MCP_PUBLIC_URL", "")),
		RequiredScopes: csv(env("MCP_REQUIRED_SCOPE", "")),

		SupabaseURL:            trimTrailingSlash(env("SUPABASE_URL", "")),
		SupabaseAnonKey:        firstNonEmpty(env("SUPABASE_ANON_KEY", ""), env("SUPABASE_PUBLISHABLE_KEY", "")),
		SupabaseOAuthProviders: csv(env("SUPABASE_OAUTH_PROVIDERS", "github,google")),

		OIDCIssuerURL:    trimTrailingSlash(env("OIDC_ISSUER_URL", "")),
		OIDCJWKSURL:      trimTrailingSlash(env("OIDC_JWKS_URL", "")),
		OIDCAudience:     env("OIDC_AUDIENCE", defaultOIDCAudience),
		OIDCAllowedEmail: env("OIDC_ALLOWED_EMAIL", ""),
		OIDCAllowedSub:   env("OIDC_ALLOWED_SUBJECT", ""),
		RequestTimeout:   durationEnv("REQUEST_TIMEOUT", defaultRequestTimeout),
		ShutdownTimeout:  durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	if cfg.OIDCIssuerURL == "" && cfg.SupabaseURL != "" {
		cfg.OIDCIssuerURL = cfg.SupabaseURL + "/auth/v1"
	}
	if cfg.OIDCJWKSURL == "" && cfg.OIDCIssuerURL != "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuerURL + "/.well-known/jwks.json"
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var errs []error
	required := map[string]string{
		"INTERVALS_ICU_API_KEY":                         c.IntervalsAPIKey,
		"INTERVALS_ICU_ATHLETE_ID":                      c.IntervalsAthleteID,
		"MCP_PUBLIC_URL":                                c.MCPPublicURL,
		"SUPABASE_URL":                                  c.SupabaseURL,
		"SUPABASE_ANON_KEY or SUPABASE_PUBLISHABLE_KEY": c.SupabaseAnonKey,
		"OIDC_ISSUER_URL":                               c.OIDCIssuerURL,
		"OIDC_JWKS_URL":                                 c.OIDCJWKSURL,
		"OIDC_AUDIENCE":                                 c.OIDCAudience,
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

func (c Config) MCPResource() string {
	return c.MCPPublicURL + "/mcp"
}

func (c Config) ResourceMetadataURL() string {
	return c.MCPPublicURL + "/.well-known/oauth-protected-resource"
}

func loadDotenv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open dotenv: %w", err)
	}
	defer file.Close()

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
		if current, exists := os.LookupEnv(key); !exists || current == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set dotenv %s: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan dotenv: %w", err)
	}
	return nil
}

func env(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return cleanValue(value)
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := env(key, "")
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
