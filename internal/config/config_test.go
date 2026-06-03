package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadDotenvAndValidate(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	body := `
INTERVALS_ICU_API_KEY=secret
INTERVALS_ICU_ATHLETE_ID=i123
MCP_PUBLIC_URL=https://mcp.example.com
SUPABASE_URL=https://project.supabase.co
SUPABASE_ANON_KEY=anon
OIDC_ALLOWED_EMAIL=me@example.com
`
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntervalsAPIKey != "secret" {
		t.Fatalf("IntervalsAPIKey = %q", cfg.IntervalsAPIKey)
	}
	if cfg.OIDCIssuerURL != "https://project.supabase.co/auth/v1" {
		t.Fatalf("OIDCIssuerURL = %q", cfg.OIDCIssuerURL)
	}
	if cfg.OIDCJWKSURL != "https://project.supabase.co/auth/v1/.well-known/jwks.json" {
		t.Fatalf("OIDCJWKSURL = %q", cfg.OIDCJWKSURL)
	}
}

func TestLoadDefaultsToNoSupabaseOAuthProviders(t *testing.T) {
	clearConfigEnv(t)
	envPath := writeDotenv(t, `
INTERVALS_ICU_API_KEY=secret
INTERVALS_ICU_ATHLETE_ID=i123
MCP_PUBLIC_URL=https://mcp.example.com
SUPABASE_URL=https://project.supabase.co
SUPABASE_ANON_KEY=anon
OIDC_ALLOWED_EMAIL=me@example.com
`)

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SupabaseOAuthProviders) != 0 {
		t.Fatalf("SupabaseOAuthProviders = %#v, want none", cfg.SupabaseOAuthProviders)
	}
}

func TestLoadParsesExplicitSupabaseOAuthProviders(t *testing.T) {
	clearConfigEnv(t)
	envPath := writeDotenv(t, `
INTERVALS_ICU_API_KEY=secret
INTERVALS_ICU_ATHLETE_ID=i123
MCP_PUBLIC_URL=https://mcp.example.com
SUPABASE_URL=https://project.supabase.co
SUPABASE_ANON_KEY=anon
SUPABASE_OAUTH_PROVIDERS=github, google,,azure
OIDC_ALLOWED_EMAIL=me@example.com
`)

	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github", "google", "azure"}
	if !slices.Equal(cfg.SupabaseOAuthProviders, want) {
		t.Fatalf("SupabaseOAuthProviders = %#v, want %#v", cfg.SupabaseOAuthProviders, want)
	}
}

func TestValidateRequiresSingleUserGate(t *testing.T) {
	cfg := Config{
		IntervalsAPIKey:    "secret",
		IntervalsAthleteID: "i123",
		IntervalsBaseURL:   "https://intervals.icu",
		MCPPublicURL:       "https://mcp.example.com",
		SupabaseURL:        "https://project.supabase.co",
		SupabaseAnonKey:    "anon",
		OIDCIssuerURL:      "https://project.supabase.co/auth/v1",
		OIDCJWKSURL:        "https://project.supabase.co/auth/v1/.well-known/jwks.json",
		OIDCAudience:       "authenticated",
		RequestTimeout:     defaultRequestTimeout,
		ShutdownTimeout:    defaultRequestTimeout,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() succeeded without allowed email or subject")
	}
}

func TestLoadIntervalsAllowsIntervalsOnlyDotenv(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	body := `
INTERVALS_ICU_API_KEY=secret
INTERVALS_ICU_ATHLETE_ID=i123
INTERVALS_ICU_BASE_URL=https://intervals.example
REQUEST_TIMEOUT=3s
`
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadIntervals(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntervalsBaseURL != "https://intervals.example" {
		t.Fatalf("IntervalsBaseURL = %q", cfg.IntervalsBaseURL)
	}
	if cfg.RequestTimeout.String() != "3s" {
		t.Fatalf("RequestTimeout = %s", cfg.RequestTimeout)
	}
}

func TestValidateIntervalsRequiresCredentials(t *testing.T) {
	cfg := Config{
		IntervalsBaseURL: "https://intervals.icu",
		RequestTimeout:   defaultRequestTimeout,
	}
	if err := cfg.ValidateIntervals(); err == nil {
		t.Fatal("ValidateIntervals() succeeded without credentials")
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"INTERVALS_ICU_API_KEY",
		"INTERVALS_ICU_ATHLETE_ID",
		"INTERVALS_ICU_BASE_URL",
		"MCP_ADDR",
		"MCP_PUBLIC_URL",
		"MCP_REQUIRED_SCOPE",
		"SUPABASE_URL",
		"SUPABASE_ANON_KEY",
		"SUPABASE_PUBLISHABLE_KEY",
		"SUPABASE_OAUTH_PROVIDERS",
		"OIDC_ISSUER_URL",
		"OIDC_JWKS_URL",
		"OIDC_AUDIENCE",
		"OIDC_ALLOWED_EMAIL",
		"OIDC_ALLOWED_SUBJECT",
		"REQUEST_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
	} {
		value, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if ok {
				if err := os.Setenv(key, value); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeDotenv(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return envPath
}
