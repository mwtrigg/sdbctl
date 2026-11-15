package ui

import "testing"

func TestValidateEntryIDRejectsCRLF(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"valid-id", true},
		{"with spaces", false},
		{"with\ttab", false},
		{"with\nnewline", false},
		{"with\rCR", false},
		{"with\r\nCRLF", false},
		{"/leading/slash", false},
		{"back\\slash", false},
		{".hidden", false},
		{"\x00null", false},
	}
	for _, tt := range tests {
		msg := validateEntryID(tt.id)
		if tt.valid && msg != "" {
			t.Errorf("validateEntryID(%q) = %q, want ''", tt.id, msg)
		}
		if !tt.valid && msg == "" {
			t.Errorf("validateEntryID(%q) = '', want error", tt.id)
		}
	}
}

func TestValidateESPPathRejectsCRLF(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{"/valid/path", true},
		{"valid/path", true}, // BLS: leading "/" is optional
		{"/path/with spaces", false},
		{"/path/with\ttab", false},
		{"/path/with\nnewline", false},
		{"/path/with\rCR", false},
		{"/path/with\r\nCRLF", false},
		{"/path/../traversal", false},
		{"/path/with\\backslash", false},
		{"/path/\x00null", false},
	}
	for _, tt := range tests {
		msg := validateESPPath(tt.path)
		if tt.valid && msg != "" {
			t.Errorf("validateESPPath(%q) = %q, want ''", tt.path, msg)
		}
		if !tt.valid && msg == "" {
			t.Errorf("validateESPPath(%q) = '', want error", tt.path)
		}
	}
}
