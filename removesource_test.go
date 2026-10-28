package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// With --rm-source the original goes once its new file is checked whole; a new file checked only
// in part keeps it, and the note tells why.
func TestRemoveSourceKeepsAPartlyCheckedOriginal(t *testing.T) {
	folder := t.TempDir()
	original := filepath.Join(folder, "Game [0100000000010000][v0].xci")
	if err := os.WriteFile(original, []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := options{rmSource: true}

	_, note, err := removeSource(o, original, switchfs.FileCheck{Partial: true}, "Game.nsp", "converted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatal("the original was deleted after a check in part")
	}
	if !strings.HasPrefix(note, "converted; ") || !strings.Contains(note, "kept") {
		t.Errorf("note %q", note)
	}

	if _, note, err = removeSource(o, original, switchfs.FileCheck{}, "Game.nsp", "converted"); err != nil || note != "converted" {
		t.Fatalf("%q %v", note, err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatal("the original stayed after a whole check")
	}
}
