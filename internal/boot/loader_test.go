package boot

import (
	"os"
	"path/filepath"
	"strings"
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

	// Parsing must not fail: systemd-boot itself ignores the bad line, and
	// the tool must still render/edit the rest of the file. The problem is
	// surfaced in Issues instead.
	cfg, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf must be lenient, got error: %v", err)
	}
	if len(cfg.Issues) == 0 {
		t.Error("expected an issue for invalid boolean value")
	}
}

func TestParseLoaderConfUnknownKeys(t *testing.T) {
	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")
	content := `# distro managed
timeout 3
secure-boot-enroll if-safe
keyfile /loader/keys/auto/KEYFILE.key
reboot-for-bitlocker yes
preferred fedora-*.conf
`
	if err := os.WriteFile(loaderConfPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf failed: %v", err)
	}
	if cfg.SecureBootEnroll != "if-safe" {
		t.Errorf("secure-boot-enroll: got %q", cfg.SecureBootEnroll)
	}
	if cfg.RebootForBitlocker == nil || !*cfg.RebootForBitlocker {
		t.Error("reboot-for-bitlocker not parsed")
	}
	if cfg.Preferred != "fedora-*.conf" {
		t.Errorf("preferred: got %q", cfg.Preferred)
	}
	// "keyfile" is not known to the tool: it must be listed as an extra,
	// flagged nowhere as fatal, and survive a round-trip verbatim.
	if len(cfg.Extras) != 1 || cfg.Extras[0].Key != "keyfile" {
		t.Fatalf("expected keyfile in Extras, got %+v", cfg.Extras)
	}

	// Editing a known field must not lose the comment or the unknown key.
	cfg.Timeout = "5"
	out := filepath.Join(tmpDir, "loader.conf.out")
	if err := cfg.Write(out); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"# distro managed", "keyfile /loader/keys/auto/KEYFILE.key", "timeout 5", "secure-boot-enroll if-safe"} {
		if !strings.Contains(s, want) {
			t.Errorf("round-trip lost %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "timeout 3") {
		t.Errorf("old timeout value survived:\n%s", s)
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

func TestWriteLoaderConfAtomicPreservesPermissions(t *testing.T) {
	cfg := &LoaderConfig{
		Timeout: "10",
		Default: "test.conf",
		Editor:  &boolFalse,
	}

	tmpDir := t.TempDir()
	loaderConfPath := filepath.Join(tmpDir, "loader.conf")

	// Create file with restricted permissions
	if err := os.WriteFile(loaderConfPath, []byte("timeout 5\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := cfg.Write(loaderConfPath); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	fi, err := os.Stat(loaderConfPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", fi.Mode().Perm())
	}

	parsed, err := ParseLoaderConf(loaderConfPath)
	if err != nil {
		t.Fatalf("ParseLoaderConf failed: %v", err)
	}
	if parsed.Timeout != "10" {
		t.Errorf("expected timeout '10', got '%s'", parsed.Timeout)
	}
}
