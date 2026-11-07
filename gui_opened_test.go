package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	wailsoptions "github.com/wailsapp/wails/v2/pkg/options"
)

// What other NXPress are started with goes to the window open, in the section it opens in, the
// paths made whole from the folder they started in; until the page takes it, once (#87, #88).
func TestOpenedGoToTheWindow(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"a.nsp", "b.nsz", "c.xci"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{}
	for _, args := range [][]string{
		{"--open", "a.nsp"},
		{"--section", "pack", filepath.Join(folder, "b.nsz"), "c.xci"},
		{filepath.Join(folder, "b.nsz")},
		{},
		{"--section", "verify", "gone.nsp"},
	} {
		a.secondInstance(wailsoptions.SecondInstanceData{Args: args, WorkingDirectory: folder})
	}
	opened := a.TakeOpened()
	want := []Opened{
		{Files: []string{filepath.Join(folder, "a.nsp")}, Section: "verify", Open: true},
		{Files: []string{filepath.Join(folder, "b.nsz"), filepath.Join(folder, "c.xci")}, Section: "pack"},
		{Files: []string{filepath.Join(folder, "b.nsz")}, Section: "decompress"},
	}
	if len(opened) != len(want) {
		t.Fatalf("%+v", opened)
	}
	for i := range want {
		if !slices.Equal(opened[i].Files, want[i].Files) || opened[i].Section != want[i].Section || opened[i].Open != want[i].Open {
			t.Errorf("%d: %+v, not %+v", i, opened[i], want[i])
		}
	}
	if again := a.TakeOpened(); len(again) != 0 {
		t.Errorf("taken twice: %+v", again)
	}
}

// The files NXPress starts with open in the section of the menu, in Verify for a double click,
// else in Compress or Decompress by what they hold.
func TestSetupOpensInTheSectionAsked(t *testing.T) {
	folder := t.TempDir()
	nsp, nsz := filepath.Join(folder, "a.nsp"), filepath.Join(folder, "b.nsz")
	for _, path := range []string{nsp, nsz} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		app  *App
		mode string
		open bool
	}{
		{&App{files: []string{nsp}}, "compress", false},
		{&App{files: []string{nsz}}, "decompress", false},
		{&App{files: []string{nsz}, open: true}, "verify", true},
		{&App{files: []string{nsp}, section: "info"}, "info", false},
		{&App{}, "", false},
	} {
		if setup := c.app.Setup(); setup.Mode != c.mode || setup.Open != c.open {
			t.Errorf("%v %q %v: %q %v", c.app.files, c.app.section, c.app.open, setup.Mode, setup.Open)
		}
	}
}
