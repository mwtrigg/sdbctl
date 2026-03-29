package boot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLoaderConf(t *testing.T) {
	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")

	content := `timeout 5
default arch.conf
editor yes
console-mode auto
auto-entries yes
auto-firmware no
`
	if err := os.WriteFile(loaderConfPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf failed: %v", err)
	}

	if cfg.Timeout != "5" {
		t.Errorf("expected timeout '5', got '%s'", cfg.Timeout)
	}
	if cfg.Default != "arch.conf" {
		t.Errorf("expected default 'arch.conf', got '%s'", cfg.Default)
	}
	if cfg.Editor == nil || !*cfg.Editor {
		t.Error("expected editor true")
	}
	if cfg.ConsoleMode != "auto" {
		t.Errorf("expected console-mode 'auto', got '%s'", cfg.ConsoleMode)
	}
	if cfg.AutoEntries == nil || !*cfg.AutoEntries {
		t.Error("expected auto-entries true")
	}
	if cfg.AutoFirmware == nil || *cfg.AutoFirmware {
		t.Error("expected auto-firmware false")
	}
}

func TestParseLoaderConfBoolVariants(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"yes", true},
		{"y", true},
		{"true", true},
		{"t", true},
		{"on", true},
		{"1", true},
		{"no", false},
		{"n", false},
		{"false", false},
		{"f", false},
		{"off", false},
		{"0", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			tmpDir := t.TempDir()
			loaderConfPath := filepath.Join(tmpDir, "loader.conf")
			content := "editor " + tt.value
			if err := os.WriteFile(loaderConfPath, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			cfg, err := ParseLoaderConf(loaderConfPath)
			if err != nil {
				t.Fatalf("ParseLoaderConf failed: %v", err)
			}
			if cfg.Editor == nil {
				t.Fatal("editor is nil")
			}
			if *cfg.Editor != tt.expected {
				t.Errorf("expected editor %v for '%s', got %v", tt.expected, tt.value, *cfg.Editor)
			}
		})
	}
}

func TestParseLoaderConfInvalidBool(t *testing.T) {
	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")
	content := "editor maybe"
	if err := os.WriteFile(loaderConfPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseLoaderConf(loaderConfPath)
	if err == nil {
		t.Error("expected error for invalid boolean")
	}
}

func TestWriteLoaderConf(t *testing.T) {
	cfg := &LoaderConfig{
		Timeout:     "10",
		Default:     "test.conf",
		Editor:      &boolFalse,
		ConsoleMode: "max",
	}

	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")

	if err := cfg.Write(loaderConfPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	parsed, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf failed: %v", err)
	}

	if parsed.Timeout != cfg.Timeout {
		t.Errorf("timeout mismatch: got %s, want %s", parsed.Timeout, cfg.Timeout)
	}
	if parsed.Default != cfg.Default {
		t.Errorf("default mismatch: got %s, want %s", parsed.Default, cfg.Default)
	}
	if parsed.Editor == nil || *parsed.Editor != *cfg.Editor {
		t.Error("editor mismatch")
	}
	if parsed.ConsoleMode != cfg.ConsoleMode {
		t.Errorf("console-mode mismatch: got %s, want %s", parsed.ConsoleMode, cfg.ConsoleMode)
	}
}

func TestParseLoaderConfComments(t *testing.T) {
	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")

	content := `# This is a comment
timeout 5
# Another comment
default test.conf
`
	if err := os.WriteFile(loaderConfPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf failed: %v", err)
	}

	if cfg.Timeout != "5" {
		t.Errorf("expected timeout '5', got '%s'", cfg.Timeout)
	}
	if cfg.Default != "test.conf" {
		t.Errorf("expected default 'test.conf', got '%s'", cfg.Default)
	}
}
