package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/settings"
)

// Any text file is not a prod.keys: one without header_key is refused, and the keys before stay.
func TestKeysNeedAHeaderKey(t *testing.T) {
	folder := t.TempDir()
	good := filepath.Join(folder, "prod.keys")
	// made up, as in the tests of switchfs: no real key
	header := strings.Repeat("0123456789abcdef", 4)
	os.WriteFile(good, []byte("header_key = "+header+"\nkey_area_key_application_00 = "+strings.Repeat("ab", 16)+"\n"), 0o600)
	notes := filepath.Join(folder, "notes.txt")
	os.WriteFile(notes, []byte("things to do\ncopy = the games\n"), 0o600)

	if _, err := loadKeys(notes); err == nil || !strings.Contains(err.Error(), "header_key") {
		t.Fatalf("a text file is taken as keys: %v", err)
	}

	a := &App{}
	if path, err := a.useKeys(good); err != nil || path != good {
		t.Fatalf("the keys are refused: %q %v", path, err)
	}
	if _, err := a.useKeys(notes); err == nil || !strings.Contains(err.Error(), "header_key") {
		t.Fatalf("a text file is taken as keys: %v", err)
	}
	if a.keys != good {
		t.Fatalf("the keys in use are %q, want the ones before", a.keys)
	}
	if keys, _ := settings.SwitchKeys(); keys == nil || keys.GetKey("header_key") != header {
		t.Fatal("the keys before are no longer active")
	}
	// a file that cannot be read leaves them too
	if _, err := a.useKeys(filepath.Join(folder, "missing.keys")); err == nil || a.keys != good {
		t.Fatalf("a missing file: %v, keys %q", err, a.keys)
	}
}
