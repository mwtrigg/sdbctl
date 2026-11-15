package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// backupDirName is the directory (relative to the boot partition root) where
// sdboot-tui keeps timestamped snapshots of files it is about to overwrite or
// delete. It lives under loader/ so it travels with the ESP, but outside
// loader/entries/ so systemd-boot never sees it.
const backupDirName = "sdboot-tui-backups"

// Backup is one snapshot of one or more files.
type Backup struct {
	// ID is the directory name, e.g. "20261114-101530".
	ID string
	// Time is when the snapshot was taken.
	Time time.Time
	// Path is the absolute directory holding the snapshot.
	Path string
	// Files are the boot-directory-relative paths captured, e.g.
	// "loader/entries/arch.conf".
	Files []string
}

// BackupRoot returns the directory holding all backups for a boot directory.
func BackupRoot(bootDir string) string {
	return filepath.Join(bootDir, "loader", backupDirName)
}

// backupID formats a timestamp into a sortable directory name.
func backupID(t time.Time) string {
	return t.Format("20060102-150405")
}

// createBackupDir claims a fresh directory for a snapshot. The timestamp has
// one-second resolution, so several changes can land in the same second;
// those get a numeric suffix so no snapshot ever overwrites another.
func createBackupDir(root, id string) (string, error) {
	dir := filepath.Join(root, id)
	for i := 1; i < 1000; i++ {
		if err := os.Mkdir(dir, 0755); err == nil {
			return dir, nil
		} else if !os.IsExist(err) {
			return "", err
		}
		dir = filepath.Join(root, fmt.Sprintf("%s-%d", id, i))
	}
	return "", fmt.Errorf("too many backups created within the same second")
}

// CreateBackup snapshots the given boot-directory-relative files. Files that
// do not exist are skipped (nothing to preserve). The returned Backup lists
// the files that were actually captured.
//
// Backups are best-effort: a failure to write the snapshot is returned as an
// error so callers can decide whether to continue with the destructive
// operation.
func CreateBackup(bootDir string, relPaths []string) (*Backup, error) {
	now := time.Now().UTC()
	id := backupID(now)

	root := BackupRoot(bootDir)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}
	dir, err := createBackupDir(root, id)
	if err != nil {
		return nil, fmt.Errorf("creating backup directory: %w", err)
	}

	b := &Backup{ID: filepath.Base(dir), Time: now, Path: dir}
	for _, rel := range relPaths {
		src := filepath.Join(bootDir, filepath.FromSlash(rel))
		if _, err := os.Stat(src); err != nil {
			continue // nothing to back up (e.g. creating a new file)
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return nil, fmt.Errorf("creating backup directory: %w", err)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("reading %s for backup: %w", rel, err)
		}
		if err := writeFileAtomic(dst, data, 0644); err != nil {
			return nil, fmt.Errorf("writing backup of %s: %w", rel, err)
		}
		b.Files = append(b.Files, rel)
	}

	if len(b.Files) == 0 {
		// Nothing was captured; drop the empty directory again.
		os.Remove(dir)
		return nil, nil
	}

	return b, nil
}

// ListBackups returns all backups for a boot directory, newest first.
func ListBackups(bootDir string) ([]Backup, error) {
	root := BackupRoot(bootDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []Backup
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		b := Backup{ID: e.Name(), Path: dir}
		if t, err := time.Parse("20060102-150405", e.Name()); err == nil {
			b.Time = t
		}
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(dir, p)
			if relErr != nil {
				return nil
			}
			b.Files = append(b.Files, filepath.ToSlash(rel))
			return nil
		})
		sort.Strings(b.Files)
		backups = append(backups, b)
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ID > backups[j].ID
	})
	return backups, nil
}

// LatestBackup returns the most recent backup, or nil if there is none.
func LatestBackup(bootDir string) (*Backup, error) {
	backups, err := ListBackups(bootDir)
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, nil
	}
	return &backups[0], nil
}

// RestoreBackup copies a snapshot back over the original files. Files that
// were deleted after the snapshot are recreated. The snapshot itself is kept,
// so a restore can be repeated.
func RestoreBackup(bootDir string, b Backup) error {
	for _, rel := range b.Files {
		src := filepath.Join(b.Path, filepath.FromSlash(rel))
		dst := filepath.Join(bootDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading backup of %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("restoring %s: %w", rel, err)
		}
		if err := writeFileAtomic(dst, data, 0644); err != nil {
			return fmt.Errorf("restoring %s: %w", rel, err)
		}
	}
	return nil
}

// Describe renders a one-line summary of a backup.
func (b Backup) Describe() string {
	return fmt.Sprintf("%s  %s", b.ID, strings.Join(b.Files, ", "))
}
