package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The files of an action are the ones given and the ones in the folders given, each once, in
// order of name; a folder of parts is one file, taken only to make it whole.
func TestInputs(t *testing.T) {
	folder := t.TempDir()
	for _, name := range []string{"b.nsp", "a.nsz", "sub/c.xci", "._b.nsp", "notes.txt", "parts.nsp/00", "parts.nsp/01"} {
		path := filepath.Join(folder, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("x"), 0o644)
	}
	in := func(names ...string) []string {
		paths := []string{}
		for _, name := range names {
			paths = append(paths, filepath.Join(folder, name))
		}
		return paths
	}
	missing := filepath.Join(folder, "missing.nsp")
	files, notFound := inputs([]string{folder, missing, filepath.Join(folder, "b.nsp")}, takes["verify"])
	if !reflect.DeepEqual(files, in("a.nsz", "b.nsp", "sub/c.xci")) || !reflect.DeepEqual(notFound, []string{missing}) {
		t.Fatalf("verify: %v, missing %v", files, notFound)
	}
	if files, _ := inputs([]string{folder}, takes["fat32"]); !reflect.DeepEqual(files, in("a.nsz", "b.nsp", "parts.nsp", "sub/c.xci")) {
		t.Fatalf("fat32: %v", files)
	}
	if files, _ := inputs([]string{filepath.Join(folder, "parts.nsp")}, takes["fat32"]); !reflect.DeepEqual(files, in("parts.nsp")) {
		t.Fatalf("a folder of parts given: %v", files)
	}
	if files, _ := inputs([]string{folder}, takes["compress"]); !reflect.DeepEqual(files, in("b.nsp", "sub/c.xci")) {
		t.Fatalf("compress: %v", files)
	}
}

// A new file goes next to its original or into the output folder, under a hidden name until
// it is checked, and replaces a file there only with -w.
func TestTargetAndFree(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "game.nsz")
	if got, err := target(options{}, path); got != path || err != nil {
		t.Fatalf("next to it: %q %v", got, err)
	}
	output := filepath.Join(folder, "out")
	if got, err := target(options{output: output}, path); got != filepath.Join(output, "game.nsz") || err != nil {
		t.Fatalf("into the output folder: %q %v", got, err)
	}
	if info, err := os.Stat(output); err != nil || !info.IsDir() {
		t.Fatal("the output folder is made")
	}
	if got := temporary(path); got != filepath.Join(folder, ".game.nsz.nxpress") {
		t.Fatalf("temporary: %q", got)
	}
	if free(options{}, path) != nil {
		t.Fatal("nothing there yet")
	}
	os.WriteFile(path, []byte("x"), 0o644)
	if free(options{}, path) == nil || free(options{overwrite: true}, path) != nil {
		t.Fatal("a file there")
	}
	written := temporary(path)
	os.WriteFile(written, []byte("new"), 0o644)
	if err := finish(options{overwrite: true}, written, path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("finished: %q", got)
	}
}

// What the titles of a file tell together: the game first, the version of its newest update,
// the name in English when there is one, and how many of each kind.
func TestDescribeTitles(t *testing.T) {
	control := func(version string, names map[string]string) *switchfs.Nacp {
		titles := map[string]switchfs.NacpTitle{}
		for language, name := range names {
			titles[language] = switchfs.NacpTitle{Title: name}
		}
		return &switchfs.Nacp{TitleName: titles, DisplayVersion: version}
	}
	pack := describeTitles([]*switchfs.ContentMetaAttributes{
		{TitleId: "0100000000011001", Type: "DLC"},
		{TitleId: "0100000000010800", Type: "UPD", Version: 131072, Ncap: control("1.2.0", map[string]string{"AmericanEnglish": "Game"})},
		{TitleId: "0100000000010000", Type: "BASE", Ncap: control("1.0.0", map[string]string{"Japanese": "ゲーム", "AmericanEnglish": " Game "})},
		{TitleId: "0100000000010800", Type: "UPD", Version: 65536, Ncap: control("1.1.0", nil)},
	})
	want := gameTitle{Name: "Game", TitleID: "0100000000010000", Type: "BASE", Titles: 4, Latest: 131072, VersionTxt: "1.2.0", Bases: 1, Updates: 2, Dlcs: 1}
	if !reflect.DeepEqual(pack, want) {
		t.Fatalf("pack: %+v", pack)
	}
	dlc := describeTitles([]*switchfs.ContentMetaAttributes{{TitleId: "0100000000011001", Type: "DLC"}})
	if dlc.VersionTxt != "1.0.0" || dlc.Name != "" || dlc.Dlcs != 1 || switchfs.PackageContents(dlc.Bases, dlc.Updates, dlc.Dlcs) != "1D" {
		t.Fatalf("a DLC: %+v", dlc)
	}
	other := describeTitles([]*switchfs.ContentMetaAttributes{{TitleId: "0100000000010000", Type: "BASE", Ncap: control("", map[string]string{"Korean": "게임"})}})
	if other.Name != "게임" {
		t.Fatalf("a name in another language: %+v", other)
	}
}

// The results are told for people, as "<percentage>% <step>" or as JSON lines; the progress
// once every few seconds, and always at 100 %.
func TestReporter(t *testing.T) {
	var text bytes.Buffer
	people := &reporter{out: &text}
	people.done("C:/games/a.nsp", "C:/games/a.nsz", "1 NCA compressed", nil)
	people.done("C:/games/b.nsp", "", "", errors.New("broken"))
	people.done("C:/games/c.nsp", "", "checked", nil)
	people.say("%d files", 3)
	want := "OK a.nsp -> a.nsz (1 NCA compressed)\nFAILED b.nsp: broken\nOK c.nsp (checked)\n3 files\n"
	if text.String() != want {
		t.Fatalf("for people: %q", text.String())
	}

	var minimal bytes.Buffer
	short := &reporter{out: &minimal, minimal: true}
	progress := short.progress("a.nsp", "Compressing")
	progress(0, 100)
	progress(50, 100)
	progress(100, 100)
	short.say("told")
	if minimal.String() != "\r| 0% Compressing\r/ 100% Compressing\r"+strings.Repeat(" ", 18)+"\rtold\n" {
		t.Fatalf("minimal: %q", minimal.String())
	}

	var machine bytes.Buffer
	(&reporter{out: &machine, machine: true}).done("a.nsp", "a.nsz", "", nil)
	if got := strings.TrimSpace(machine.String()); got != `{"type": "complete", "file": "a.nsp", "result": "ok", "output": "a.nsz"}` {
		t.Fatalf("machine: %s", got)
	}
}
