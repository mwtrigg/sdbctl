package boot

import (
	"encoding/binary"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// oneShotVarPath is the efivars sysfs path for the LoaderEntryOneShot variable.
// The GUID is the systemd EFI variable namespace.
const oneShotVarPath = "/sys/firmware/efi/efivars/LoaderEntryOneShot-4a67b082-0a4c-41cf-b6c7-440b29bb8c4f"

// lastBootedVarPath holds the full ID of the entry that was booted most
// recently when default @saved is in effect (written by systemd-boot ≥ v253).
const lastBootedVarPath = "/sys/firmware/efi/efivars/LoaderEntryLastBooted-4a67b082-0a4c-41cf-b6c7-440b29bb8c4f"

// efiVarAttrs is the standard attribute set for a runtime EFI variable:
// EFI_VARIABLE_NON_VOLATILE | EFI_VARIABLE_BOOTSERVICE_ACCESS | EFI_VARIABLE_RUNTIME_ACCESS
const efiVarAttrs uint32 = 0x07

// Linux ioctl constants for file-attribute manipulation (FS_IOC_*).
const (
	fsIocGetFlags = 0x80086601
	fsIocSetFlags = 0x40086602
	fsImmutableFL = 0x00000010
)

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

// GetOneShot reads the current LoaderEntryOneShot EFI variable and returns the
// full entry ID stored there (file name including .conf/.efi suffix), or "" if
// the variable is not set. systemd-boot matches this value as a glob against
// full entry IDs, so suffixed IDs are the safe form to write.
func GetOneShot() (string, error) {
	return readVarString(oneShotVarPath)
}

// GetLastBooted reads the LoaderEntryLastBooted EFI variable and returns the
// full entry ID of the most recent @saved-style boot, or "" if unset
// (also unset on systemd-boot < v253 or when @saved is not in use).
func GetLastBooted() (string, error) {
	return readVarString(lastBootedVarPath)
}

// SetOneShot writes entryID as the LoaderEntryOneShot EFI variable so that
// systemd-boot boots it exactly once on the next restart. entryID should be
// the full entry ID (including suffix); see GetOneShot.
func SetOneShot(entryID string) error {
	// If the variable already exists, clear its immutable flag first.
	if _, err := os.Stat(oneShotVarPath); err == nil {
		if err := clearImmutable(oneShotVarPath); err != nil {
			return err
		}
	}
	u16 := utf16.Encode([]rune(entryID))
	payload := make([]byte, 4+len(u16)*2+2) // attrs + UTF-16LE + null terminator
	binary.LittleEndian.PutUint32(payload, efiVarAttrs)
	for i, w := range u16 {
		binary.LittleEndian.PutUint16(payload[4+i*2:], w)
	}
	// Last two bytes are zero (null terminator), already set by make.
	return os.WriteFile(oneShotVarPath, payload, 0644)
}

// ClearOneShot removes the LoaderEntryOneShot EFI variable.
func ClearOneShot() error {
	if _, err := os.Stat(oneShotVarPath); os.IsNotExist(err) {
		return nil
	}
	if err := clearImmutable(oneShotVarPath); err != nil {
		return err
	}
	err := os.Remove(oneShotVarPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
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
		return errno
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
	_, err := os.Stat("/sys/firmware/efi/efivars")
	return err == nil
}
