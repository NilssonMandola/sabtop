package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// isolate points the config file at a temp dir and clears the environment so
// each case starts from a known state.
func isolate(t *testing.T, fileBody string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("SAB_URL", "")
	t.Setenv("SAB_API_KEY", "")
	os.Unsetenv("SAB_URL")
	os.Unsetenv("SAB_API_KEY")

	if fileBody != "" {
		if err := os.MkdirAll(filepath.Join(dir, "sabtop"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sabtop", "config"), []byte(fileBody), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadRequiresToken(t *testing.T) {
	isolate(t, "")
	if _, err := Load("", ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestLoadFromFile(t *testing.T) {
	isolate(t, "# comment\n\nurl = http://nas:8080\ntoken = \"abc123\"\n")
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://nas:8080" {
		t.Errorf("URL = %q", cfg.URL)
	}
	if cfg.Token != "abc123" {
		t.Errorf("Token = %q, quotes should be stripped", cfg.Token)
	}
}

func TestPrecedence(t *testing.T) {
	isolate(t, "url = http://from-file:8080\ntoken = file-token\n")
	t.Setenv("SAB_URL", "http://from-env:8080")
	t.Setenv("SAB_API_KEY", "env-token")

	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://from-env:8080" || cfg.Token != "env-token" {
		t.Errorf("environment should beat the file, got %q / %q", cfg.URL, cfg.Token)
	}

	cfg, err = Load("http://from-flag:8080", "flag-token")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://from-flag:8080" || cfg.Token != "flag-token" {
		t.Errorf("flags should beat everything, got %q / %q", cfg.URL, cfg.Token)
	}
}

func TestSchemeIsAdded(t *testing.T) {
	isolate(t, "")
	cfg, err := Load("nas.local:8080", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://nas.local:8080" {
		t.Errorf("URL = %q, want an http:// prefix", cfg.URL)
	}
}

func TestHTTPSIsPreserved(t *testing.T) {
	isolate(t, "")
	cfg, err := Load("https://sab.example.com", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "https://sab.example.com" {
		t.Errorf("URL = %q, https should survive untouched", cfg.URL)
	}
}

func TestDefaultURLWhenOnlyTokenGiven(t *testing.T) {
	isolate(t, "")
	cfg, err := Load("", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != DefaultURL {
		t.Errorf("URL = %q, want %q", cfg.URL, DefaultURL)
	}
}

func TestMalformedLineIsReported(t *testing.T) {
	isolate(t, "token = abc\nthis line has no equals sign\n")
	if _, err := Load("", ""); err == nil {
		t.Fatal("expected a parse error")
	}
}
