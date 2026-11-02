package jobs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/system"
	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// renameNew never replaces a file or a folder: it gives the name only when nothing has it.
func TestRenameNewNeverReplaces(t *testing.T) {
	folder := t.TempDir()
	a := filepath.Join(folder, "a")
	b := filepath.Join(folder, "b")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	if err := system.RenameNew(a, b); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("onto a file: %v", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" || !testkit.Exists(a) {
		t.Fatalf("b holds %q, a is there %v", got, testkit.Exists(a))
	}
	c := filepath.Join(folder, "c")
	if err := system.RenameNew(a, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(c); string(got) != "a" || testkit.Exists(a) {
		t.Fatalf("c holds %q, a is there %v", got, testkit.Exists(a))
	}
	parts := filepath.Join(folder, "parts")
	os.Mkdir(parts, 0o755)
	if err := system.RenameNew(parts, b); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("a folder onto a file: %v", err)
	}
	if err := system.RenameNew(parts, filepath.Join(folder, "moved")); err != nil {
		t.Fatalf("a folder: %v", err)
	}
}

// A file whose new name is taken by a file outside the work keeps its name without -w, and
// takes the place of that file only with -w; a new name that changes only the case is given.
func TestRenameReplacesOnlyWhenAsked(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	game := testkit.TitleNsp(t, filepath.Join(folder, "game [x].nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	taken := filepath.Join(folder, "BASE.nsp")
	os.WriteFile(taken, []byte("another file"), 0o644)

	out := &testkit.Results{}
	RunJobs(context.Background(), Options{Template: "{type}", Files: []string{game}, Multi: 1, Threads: 1}, out)
	if got, _ := os.ReadFile(taken); string(got) != "another file" || !testkit.Exists(game) {
		t.Fatalf("without -w: the file there holds %q, the game is there %v", got, testkit.Exists(game))
	}
	if out.Results[game].Err == nil {
		t.Fatal("renamed onto a file there without -w")
	}

	RunJobs(context.Background(), Options{Template: "{type}", Overwrite: true, Files: []string{game}, Multi: 1, Threads: 1}, testkit.Quiet{})
	if testkit.Exists(game) {
		t.Fatal("with -w the game keeps its name")
	}
	if got := safetyTitles(t, taken); len(got) != 1 {
		t.Fatalf("with -w: %v", got)
	}

	lower := filepath.Join(folder, "base.nsp")
	if err := os.Rename(taken, lower); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rename(context.Background(), lower, Options{Template: "{type}"}, testkit.Quiet{}); err != nil {
		t.Fatalf("a new case: %v", err)
	}
	if names := testkit.List(folder); len(names) != 1 || names[0] != "BASE.nsp" {
		t.Fatalf("a new case: %v", names)
	}
}
