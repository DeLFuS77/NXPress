package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The backup is checked before the secret is set: a folder that is not there, a file that is
// there already or a place inside the repository stop everything first.
func TestCheckBackup(t *testing.T) {
	repository := t.TempDir()
	outside := t.TempDir()
	taken := filepath.Join(outside, "taken.key")
	os.WriteFile(taken, []byte("x"), 0o600)
	for path, ok := range map[string]bool{
		filepath.Join(outside, "nxpress-signing.key"):    true,
		filepath.Join(outside, "missing", "nxpress.key"): false,
		taken:                                    false,
		filepath.Join(repository, "nxpress.key"): false,
		filepath.Join(repository, "internal", "nxpress.key"): false,
	} {
		if err := checkBackup(path, repository); (err == nil) != ok {
			t.Errorf("%s: %v", path, err)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Errorf("left in the folder: %v", entries)
	}
}
