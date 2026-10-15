package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvSetsMissingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "API_ENV=dev\nAPI_PORT=19999\n# comment\n\nAPI_LLM_PROVIDER=\"mock\"\nexport API_REDIS_ADDR='127.0.0.1:19999'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("API_ENV", "prod")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv() error: %v", err)
	}

	if got := os.Getenv("API_ENV"); got != "prod" {
		t.Errorf("existing env was overridden: API_ENV=%q", got)
	}
	if got := os.Getenv("API_PORT"); got != "19999" {
		t.Errorf("API_PORT = %q, want 19999", got)
	}
	if got := os.Getenv("API_LLM_PROVIDER"); got != "mock" {
		t.Errorf("quoted value not parsed: %q", got)
	}
	if got := os.Getenv("API_REDIS_ADDR"); got != "127.0.0.1:19999" {
		t.Errorf("export prefix not stripped: %q", got)
	}
}

func TestFindDotEnvWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("X=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	found, ok := FindDotEnv(nested)
	if !ok {
		t.Fatal("FindDotEnv() did not find the file")
	}
	if want := filepath.Join(root, ".env"); filepath.Clean(found) != filepath.Clean(want) {
		t.Errorf("found %q, want %q", found, want)
	}

	empty := t.TempDir()
	if _, ok := FindDotEnv(empty); ok {
		t.Error("FindDotEnv() found a file in an empty tree")
	}
}
