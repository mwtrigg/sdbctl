package boot

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// loaderGUID is the systemd EFI variable namespace.
const loaderGUID = "4a67b082-0a4c-41cf-b6c7-440b29bb8c4f"

// efiVarAttrs is the standard attribute set for a runtime EFI variable:
// EFI_VARIABLE_NON_VOLATILE | EFI_VARIABLE_BOOTSERVICE_ACCESS | EFI_VARIABLE_RUNTIME_ACCESS
const efiVarAttrs uint32 = 0x07

// Linux ioctl constants for file-attribute manipulation (FS_IOC_*).
const (
	fsIocGetFlags = 0x80086601
	fsIocSetFlags = 0x40086602
	fsImmutableFL = 0x00000010
)

// efivarsDir returns the efivarfs mount point. SDBCTL_EFIVARS overrides it,
// which matters in three situations: a chroot or container where /sys is not
// bind-mounted, a mount at a non-standard path, and the test suite (which has
// no EFI at all). The override is documented in the man page.
func efivarsDir() string {
	if d := os.Getenv("SDBCTL_EFIVARS"); d != "" {
		return d
	}
	return "/sys/firmware/efi/efivars"
}

// loaderVarPath builds the sysfs path for one variable in the loader GUID
// namespace, matching the efivarfs "<Name>-<GUID>" convention.
func loaderVarPath(name string) string {
	return filepath.Join(efivarsDir(), name+"-"+loaderGUID)
}

// readVarString reads an efivars sysfs file holding a NUL-terminated UTF-16LE
// string (4-byte attributes header + payload). Returns "" if the variable
// does not exist.
func readVarString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	// EFI variable file layout: 4-byte attributes header + UTF-16LE payload.
	if len(data) < 6 {
		return "", nil
	}
	payload := data[4:]
	// Strip trailing null words.
	for len(payload) >= 2 && payload[len(payload)-2] == 0 && payload[len(payload)-1] == 0 {
		payload = payload[:len(payload)-2]
	}
	if len(payload)%2 != 0 {
		payload = payload[:len(payload)-1]
	}
	u16 := make([]uint16, len(payload)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(payload[i*2:])
	}
	return string(utf16.Decode(u16)), nil
}

// writeVarString stores value as a NUL-terminated UTF-16LE string under the
// loader GUID namespace, clearing the immutable flag efivarfs sets.
func writeVarString(path, value string) error {
	if _, err := os.Stat(path); err == nil {
		if err := clearImmutable(path); err != nil {
			return err
		}
	}
	u16 := utf16.Encode([]rune(value))
	payload := make([]byte, 4+len(u16)*2+2) // attrs + UTF-16LE + null terminator
	binary.LittleEndian.PutUint32(payload, efiVarAttrs)
	for i, w := range u16 {
		binary.LittleEndian.PutUint16(payload[4+i*2:], w)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0644)
}

// GetOneShot reads the current LoaderEntryOneShot EFI variable and returns the
// full entry ID stored there (file name including .conf/.efi suffix), or "" if
// the variable is not set. systemd-boot matches this value as a glob against
// full entry IDs, so suffixed IDs are the safe form to write.
func GetOneShot() (string, error) {
	return readVarString(loaderVarPath("LoaderEntryOneShot"))
}

// SetOneShot writes entryID as the LoaderEntryOneShot EFI variable so that
// systemd-boot boots it exactly once on the next restart. entryID should be
// the full entry ID (including suffix); see GetOneShot.
func SetOneShot(entryID string) error {
	return writeVarString(loaderVarPath("LoaderEntryOneShot"), entryID)
}

// ClearOneShot removes the LoaderEntryOneShot EFI variable.
func ClearOneShot() error {
	path := loaderVarPath("LoaderEntryOneShot")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := clearImmutable(path); err != nil {
		return err
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetLastBooted reads the LoaderEntryLastBooted EFI variable and returns the
// full entry ID of the most recent @saved-style boot, or "" if unset
// (also unset on systemd-boot < 253 or when @saved is not in use).
func GetLastBooted() (string, error) {
	return readVarString(loaderVarPath("LoaderEntryLastBooted"))
}

// clearImmutable removes the FS_IMMUTABLE_FL attribute from path so that the
// kernel allows subsequent writes. efivars files have this flag set by default.
func clearImmutable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var flags int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		fsIocGetFlags, uintptr(unsafe.Pointer(&flags))); errno != 0 {
		// Not a filesystem that supports flags (e.g. a test fixture): nothing
		// to clear.
		return nil
	}
	flags &^= fsImmutableFL
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		fsIocSetFlags, uintptr(unsafe.Pointer(&flags))); errno != 0 {
		return errno
	}
	return nil
}

// OneShotAvailable reports whether the EFI variable interface is accessible.
func OneShotAvailable() bool {
	_, err := os.Stat(efivarsDir())
	return err == nil
}

// EfivarsDir exposes the effective efivarfs location for status output.
func EfivarsDir() string { return efivarsDir() }
