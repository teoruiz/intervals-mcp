package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenvAndValidate(t *testing.T) {
	t.Setenv("INTERVALS_ICU_API_KEY", "")
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
