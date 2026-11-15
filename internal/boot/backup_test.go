package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateBackupAndRestore(t *testing.T) {
	esp := t.TempDir()
	entriesDir := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(entriesDir, "arch.conf")
	original := "title Arch\nlinux /vmlinuz\n"
	if err := os.WriteFile(entryPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := CreateBackup(esp, []string{"loader/entries/arch.conf"})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected a backup")
	}
	if len(b.Files) != 1 || b.Files[0] != "loader/entries/arch.conf" {
		t.Fatalf("unexpected backup contents: %+v", b)
	}

	// The snapshot must not live where systemd-boot reads entries from.
	if strings.HasPrefix(b.Path, entriesDir) {
		t.Errorf("backup stored inside loader/entries: %s", b.Path)
	}
	if _, err := os.Stat(filepath.Join(esp, "loader", "sdboot-tui-backups")); err != nil {
		t.Errorf("expected backup root: %v", err)
	}

	// Modify the file, then restore.
	if err := os.WriteFile(entryPath, []byte("title Broken\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreBackup(esp, *b); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	data, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Errorf("restore did not restore content:\n%s", data)
	}
}

func TestRestoreRecoversDeletedFile(t *testing.T) {
	esp := t.TempDir()
	entriesDir := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(entriesDir, "gone.conf")
	if err := os.WriteFile(entryPath, []byte("title Gone\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := CreateBackup(esp, []string{"loader/entries/gone.conf"})
	if err != nil || b == nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if err := os.Remove(entryPath); err != nil {
		t.Fatal(err)
	}
	if err := RestoreBackup(esp, *b); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if _, err := os.Stat(entryPath); err != nil {
		t.Fatalf("deleted file not restored: %v", err)
	}
}

func TestCreateBackupSkipsMissingFiles(t *testing.T) {
	esp := t.TempDir()
	b, err := CreateBackup(esp, []string{"loader/entries/nope.conf"})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}
	if b != nil {
		t.Errorf("expected no backup for missing file, got %+v", b)
	}
}

func TestListBackupsNewestFirst(t *testing.T) {
	esp := t.TempDir()
	entriesDir := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.conf", "b.conf"} {
		if err := os.WriteFile(filepath.Join(entriesDir, name), []byte("title x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := CreateBackup(esp, []string{"loader/entries/a.conf"}); err != nil {
		t.Fatal(err)
	}
	// The directory name is second-resolution, so create a second snapshot
	// with a distinct, older ID to check ordering.
	older := filepath.Join(BackupRoot(esp), "20200101-000000")
	if err := os.MkdirAll(older, 0755); err != nil {
		t.Fatal(err)
	}

	backups, err := ListBackups(esp)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	if backups[0].ID <= backups[1].ID {
		t.Errorf("backups not sorted newest first: %v", backups)
	}

	latest, err := LatestBackup(esp)
	if err != nil || latest == nil {
		t.Fatalf("LatestBackup failed: %v", err)
	}
	if latest.ID != backups[0].ID {
		t.Errorf("LatestBackup = %q, want %q", latest.ID, backups[0].ID)
	}
}

func TestBuildReport(t *testing.T) {
	esp := t.TempDir()
	entriesDir := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(esp, "loader", "loader.conf"), []byte("default arch.conf\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entriesDir, "arch.conf"),
		[]byte("title Arch\nlinux /missing-kernel\n"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := BuildReport(esp)
	if err != nil {
		t.Fatalf("BuildReport failed: %v", err)
	}
	if report.OK() {
		t.Error("expected problems (missing kernel file)")
	}
	if len(report.EntryIssues["arch"]) == 0 {
		t.Errorf("expected entry issues, got %v", report.EntryIssues)
	}

	text := report.Text()
	for _, want := range []string{"sdboot-tui diagnostics", "boot directory", "loader.conf", "missing-kernel", "problem(s) found"} {
		if !strings.Contains(text, want) {
			t.Errorf("report text missing %q:\n%s", want, text)
		}
	}
}

func TestCreateBackupSameSecondDoesNotOverwrite(t *testing.T) {
	esp := t.TempDir()
	entriesDir := filepath.Join(esp, "loader", "entries")
	if err := os.MkdirAll(entriesDir, 0755); err != nil {
		t.Fatal(err)
	}
	entryPath := filepath.Join(entriesDir, "arch.conf")
	if err := os.WriteFile(entryPath, []byte("title One\n"), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := CreateBackup(esp, []string{"loader/entries/arch.conf"})
	if err != nil || first == nil {
		t.Fatalf("first backup failed: %v", err)
	}

	// Change the file, then snapshot again — most likely within the same
	// second. The first snapshot must survive.
	if err := os.WriteFile(entryPath, []byte("title Two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := CreateBackup(esp, []string{"loader/entries/arch.conf"})
	if err != nil || second == nil {
		t.Fatalf("second backup failed: %v", err)
	}
	if first.Path == second.Path {
		t.Fatalf("second snapshot reused the first directory: %s", first.Path)
	}

	data, err := os.ReadFile(filepath.Join(first.Path, "loader", "entries", "arch.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "title One\n" {
		t.Errorf("first snapshot was overwritten: %q", data)
	}
}
