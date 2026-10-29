package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// renameNew never replaces a file or a folder: it gives the name only when nothing has it.
func TestRenameNewNeverReplaces(t *testing.T) {
	folder := t.TempDir()
	a := filepath.Join(folder, "a")
	b := filepath.Join(folder, "b")
	os.WriteFile(a, []byte("a"), 0o644)
	os.WriteFile(b, []byte("b"), 0o644)
	if err := renameNew(a, b); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("onto a file: %v", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" || !safetyExists(a) {
		t.Fatalf("b holds %q, a is there %v", got, safetyExists(a))
	}
	c := filepath.Join(folder, "c")
	if err := renameNew(a, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(c); string(got) != "a" || safetyExists(a) {
		t.Fatalf("c holds %q, a is there %v", got, safetyExists(a))
	}
	parts := filepath.Join(folder, "parts")
	os.Mkdir(parts, 0o755)
	if err := renameNew(parts, b); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("a folder onto a file: %v", err)
	}
	if err := renameNew(parts, filepath.Join(folder, "moved")); err != nil {
		t.Fatalf("a folder: %v", err)
	}
}

// A file whose new name is taken by a file outside the work keeps its name without -w, and
// takes the place of that file only with -w; a new name that changes only the case is given.
func TestRenameReplacesOnlyWhenAsked(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	game := safetyTitleNsp(t, filepath.Join(folder, "game [x].nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	taken := filepath.Join(folder, "BASE.nsp")
	os.WriteFile(taken, []byte("another file"), 0o644)

	out := &safetyResults{}
	runJobs(context.Background(), options{template: "{type}", files: []string{game}, multi: 1, threads: 1}, out)
	if got, _ := os.ReadFile(taken); string(got) != "another file" || !safetyExists(game) {
		t.Fatalf("without -w: the file there holds %q, the game is there %v", got, safetyExists(game))
	}
	if out.results[game].err == nil {
		t.Fatal("renamed onto a file there without -w")
	}

	runJobs(context.Background(), options{template: "{type}", overwrite: true, files: []string{game}, multi: 1, threads: 1}, quiet{})
	if safetyExists(game) {
		t.Fatal("with -w the game keeps its name")
	}
	if got := safetyTitles(t, taken); len(got) != 1 {
		t.Fatalf("with -w: %v", got)
	}

	lower := filepath.Join(folder, "base.nsp")
	if err := os.Rename(taken, lower); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rename(context.Background(), lower, options{template: "{type}"}, quiet{}); err != nil {
		t.Fatalf("a new case: %v", err)
	}
	if names := safetyList(folder); len(names) != 1 || names[0] != "BASE.nsp" {
		t.Fatalf("a new case: %v", names)
	}
}
