package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigShowRedactsSecrets(t *testing.T) {
	clearRunnerEnv(t)
	dir := t.TempDir()
	xdgRoot := filepath.Join(dir, "xdg")
	configDir := filepath.Join(xdgRoot, "intervals-mcp")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	if err := os.WriteFile(filepath.Join(configDir, "config.env"), []byte(`
INTERVALS_ICU_API_KEY=super-secret
INTERVALS_ICU_ATHLETE_ID=i123
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Run(context.Background(), RunOptions{
		Args:       []string{"config", "show"},
		BinaryName: "intervals",
		Out:        &out,
		ErrOut:     ioDiscard{},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := out.String()
	if strings.Contains(output, "super-secret") {
		t.Fatalf("config show leaked secret: %s", output)
	}
	if !strings.Contains(output, "INTERVALS_ICU_API_KEY=REDACTED") {
		t.Fatalf("config show did not redact API key: %s", output)
	}
}

func TestRunCredentialCommandWithoutConfigNonInteractive(t *testing.T) {
	clearRunnerEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))

	var out bytes.Buffer
	err := Run(context.Background(), RunOptions{
		Args:       []string{"today"},
		BinaryName: "intervals",
		In:         strings.NewReader(""),
		Out:        &out,
		ErrOut:     ioDiscard{},
		WorkDir:    dir,
	})
	if err == nil {
		t.Fatal("Run succeeded without config")
	}
	if !strings.Contains(err.Error(), "intervals config init") {
		t.Fatalf("error = %v, want config init guidance", err)
	}
}

func TestRunConfigPathUsesExplicitEnv(t *testing.T) {
	clearRunnerEnv(t)
	var out bytes.Buffer
	err := Run(context.Background(), RunOptions{
		Args:       []string{"--env", "custom.env", "config", "path"},
		BinaryName: "intervals",
		Out:        &out,
		ErrOut:     ioDiscard{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "custom.env" {
		t.Fatalf("output = %q, want custom.env", out.String())
	}
}

func clearRunnerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"INTERVALS_ICU_API_KEY",
		"INTERVALS_ICU_ATHLETE_ID",
		"INTERVALS_ICU_BASE_URL",
		"MCP_ADDR",
		"REQUEST_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}
