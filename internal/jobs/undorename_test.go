package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

// Undo of a Rename gives a file its old name back, never over another file that took it, and
// says so when the file renamed is gone.
func TestUndoRename(t *testing.T) {
	dir := t.TempDir()
	old, renamed := filepath.Join(dir, "game.nsp"), filepath.Join(dir, "Made-up Quest [0100000000010000][v0].nsp")
	if err := os.WriteFile(renamed, []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UndoRename(old, renamed); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "game" || exists(renamed) {
		t.Fatal("the file does not have its old name back")
	}

	// another file took the old name meanwhile: it is kept, and the file keeps its new name
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UndoRename(old, renamed); err == nil {
		t.Fatal("another file with the old name is replaced")
	}
	if data, _ := os.ReadFile(old); string(data) != "other" || !exists(renamed) {
		t.Fatal("a file was lost")
	}

	// the file renamed is no longer there
	os.Remove(renamed)
	if err := UndoRename(filepath.Join(dir, "gone.nsp"), renamed); err == nil {
		t.Fatal("a file gone is not told")
	}
}
