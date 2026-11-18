package trash

import (
	"os"
	"path/filepath"
	"testing"
)

// An item goes into the Trash under its name, and one of a name it holds already gets a number,
// as the Finder names it; names in other alphabets and folders go as they are.
func TestPutIntoTheTrash(t *testing.T) {
	trash := t.TempDir()
	folder := t.TempDir()
	for i, name := range []string{"Game.nsp", "Game.nsp", "ゲーム ñ.nsz", "Game dump"} {
		path := filepath.Join(folder, name)
		if name == "Game dump" {
			if err := os.MkdirAll(filepath.Join(path, "base"), 0o755); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := put(trash, path); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", name)
		}
	}
	for _, name := range []string{"Game.nsp", "Game 2.nsp", "ゲーム ñ.nsz", filepath.Join("Game dump", "base")} {
		if _, err := os.Stat(filepath.Join(trash, name)); err != nil {
			t.Error(err)
		}
	}
}

// A file of the temporary folder, on the volume of the home folder, goes to ~/.Trash: only told
// here, not moved, to leave the Trash of the person who runs the tests as it is.
func TestCheckTheHomeVolume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.nsp")
	if err := os.WriteFile(path, []byte("game"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err != nil {
		if _, ok := Why(err); !ok {
			t.Fatal(err)
		}
	}
	if numbered("Game.nsp", 3) != "Game 3.nsp" || numbered("Game dump", 2) != "Game dump 2" {
		t.Error(numbered("Game.nsp", 3), numbered("Game dump", 2))
	}
}
