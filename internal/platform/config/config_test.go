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

	cfg, _, err := LoadDiscovered(DiscoveryOptions{EnvPath: envPath, EnvExplicit: true})
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

	cfg, _, err := LoadDiscovered(DiscoveryOptions{EnvPath: envPath, EnvExplicit: true})
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

	cfg, _, err := LoadDiscovered(DiscoveryOptions{EnvPath: envPath, EnvExplicit: true})
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

	cfg, _, err := LoadIntervalsDiscovered(DiscoveryOptions{EnvPath: envPath, EnvExplicit: true})
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

func TestLoadIntervalsProcessEnvOverridesDotenv(t *testing.T) {
	clearConfigEnv(t)
	envPath := writeDotenv(t, `
INTERVALS_ICU_API_KEY=file-secret
INTERVALS_ICU_ATHLETE_ID=file-athlete
`)
	t.Setenv("INTERVALS_ICU_API_KEY", "env-secret")

	cfg, _, err := LoadIntervalsDiscovered(DiscoveryOptions{EnvPath: envPath, EnvExplicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IntervalsAPIKey != "env-secret" {
		t.Fatalf("IntervalsAPIKey = %q, want env-secret", cfg.IntervalsAPIKey)
	}
	if cfg.IntervalsAthleteID != "file-athlete" {
		t.Fatalf("IntervalsAthleteID = %q, want file-athlete", cfg.IntervalsAthleteID)
	}
}

func TestLoadIntervalsDiscoveredPrefersExplicitThenXDGThenLocal(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	xdgRoot := filepath.Join(dir, "xdg")
	workDir := filepath.Join(dir, "work")
	if err := os.MkdirAll(filepath.Join(xdgRoot, "intervals-mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	xdgPath := filepath.Join(xdgRoot, "intervals-mcp", "config.env")
	localPath := filepath.Join(workDir, ".env")
	explicitPath := filepath.Join(dir, "explicit.env")
	writeFile(t, xdgPath, "INTERVALS_ICU_API_KEY=xdg\nINTERVALS_ICU_ATHLETE_ID=xdg-athlete\n")
	writeFile(t, localPath, "INTERVALS_ICU_API_KEY=local\nINTERVALS_ICU_ATHLETE_ID=local-athlete\n")
	writeFile(t, explicitPath, "INTERVALS_ICU_API_KEY=explicit\nINTERVALS_ICU_ATHLETE_ID=explicit-athlete\n")

	cfg, source, err := LoadIntervalsDiscovered(DiscoveryOptions{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if source.Kind != SourceXDG || cfg.IntervalsAPIKey != "xdg" {
		t.Fatalf("source=%+v api=%q, want xdg", source, cfg.IntervalsAPIKey)
	}

	cfg, source, err = LoadIntervalsDiscovered(DiscoveryOptions{
		EnvPath:     explicitPath,
		EnvExplicit: true,
		WorkDir:     workDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Kind != SourceExplicit || cfg.IntervalsAPIKey != "explicit" {
		t.Fatalf("source=%+v api=%q, want explicit", source, cfg.IntervalsAPIKey)
	}
}

func TestLoadIntervalsDiscoveredFallsBackToLocalDotenv(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	xdgRoot := filepath.Join(dir, "xdg")
	workDir := filepath.Join(dir, "work")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	localPath := filepath.Join(workDir, ".env")
	writeFile(t, localPath, "INTERVALS_ICU_API_KEY=local\nINTERVALS_ICU_ATHLETE_ID=local-athlete\n")

	cfg, source, err := LoadIntervalsDiscovered(DiscoveryOptions{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if source.Kind != SourceLocal || cfg.IntervalsAPIKey != "local" {
		t.Fatalf("source=%+v api=%q, want local", source, cfg.IntervalsAPIKey)
	}
}

func TestLoadIntervalsDiscoveredErrorsForMissingExplicitEnv(t *testing.T) {
	clearConfigEnv(t)
	_, _, err := LoadIntervalsDiscovered(DiscoveryOptions{
		EnvPath:     filepath.Join(t.TempDir(), "missing.env"),
		EnvExplicit: true,
	})
	if err == nil {
		t.Fatal("LoadIntervalsDiscovered succeeded with missing explicit env file")
	}
}

func TestWriteIntervalsConfigCreates0600File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intervals-mcp", "config.env")
	if err := WriteIntervalsConfig(path, IntervalsCredentials{
		APIKey:    "secret",
		AthleteID: "i123",
		BaseURL:   "https://intervals.icu",
	}, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
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

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
