package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// quiet takes the progress and the results of a test without printing them.
type quiet struct{}

func (quiet) progress(string, string) func(int64, int64) { return func(int64, int64) {} }
func (quiet) done(string, string, string, error)         {}
func (quiet) say(string, ...any)                         {}

// A file bigger than a part goes to a folder of parts with its name, read as one file, and
// back to one file again, in place.
func TestFat32SplitsAndJoinsInPlace(t *testing.T) {
	defer func(size int64) { fat32Part = size }(fat32Part)
	fat32Part = 10
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "Game [0100000000010000][v0].nsp")
	data := []byte("0123456789abcdefghijKLMNO")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	name, note, err := splitParts(ctx, path, int64(len(data)), options{}, quiet{})
	if err != nil || name != path || !strings.HasPrefix(note, "3 parts in a folder") {
		t.Fatalf("split: %q %q %v", name, note, err)
	}
	for i, want := range []string{"0123456789", "abcdefghij", "KLMNO"} {
		if got, err := os.ReadFile(filepath.Join(path, fmt.Sprintf("%02d", i))); err != nil || string(got) != want {
			t.Fatalf("part %d: %q %v", i, got, err)
		}
	}
	if whole, ok := splitContainer(path); !ok || whole != path {
		t.Fatalf("a folder of parts: %q %v", whole, ok)
	}
	if size, parts, err := partsSize(path); size != int64(len(data)) || parts != 3 || err != nil {
		t.Fatalf("size of the parts: %d %d %v", size, parts, err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("the temporary folder is left: %v", entries)
	}

	name, note, err = fat32(ctx, path, options{}, quiet{})
	if err != nil || name != path || note != "3 parts joined" {
		t.Fatalf("join: %q %q %v", name, note, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("joined: %q %v", got, err)
	}
}

// Parts named name.ns0, name.ns1... are joined into name.nsp, and the parts deleted.
func TestFat32JoinsNamedParts(t *testing.T) {
	folder := t.TempDir()
	first := filepath.Join(folder, "Game.ns0")
	for i, part := range []string{"0123456789", "abcdefghij", "KLM"} {
		os.WriteFile(filepath.Join(folder, fmt.Sprintf("Game.ns%d", i)), []byte(part), 0o644)
	}
	if whole, ok := splitContainer(first); !ok || whole != filepath.Join(folder, "Game.nsp") {
		t.Fatalf("first part: %q %v", whole, ok)
	}
	for path, want := range map[string]bool{"Game.ns1": true, "Game.NZ2": true, "Game.ns0": false, "Game.nsp": false, "Game.nsx": false} {
		if splitPart(path) != want {
			t.Errorf("splitPart(%s) is not %v", path, want)
		}
	}
	name, note, err := fat32(context.Background(), first, options{}, quiet{})
	if err != nil || name != filepath.Join(folder, "Game.nsp") || note != "3 parts joined" {
		t.Fatalf("join: %q %q %v", name, note, err)
	}
	if got, _ := os.ReadFile(name); string(got) != "0123456789abcdefghijKLM" {
		t.Fatalf("joined: %q", got)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Fatalf("the parts are left: %v", entries)
	}
}

// Into another folder, the original stays unless asked to go, and a file there is replaced
// only with -w.
func TestFat32IntoAnotherFolder(t *testing.T) {
	defer func(size int64) { fat32Part = size }(fat32Part)
	fat32Part = 4
	folder := t.TempDir()
	path := filepath.Join(folder, "Game.xci")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	output := filepath.Join(folder, "sd")

	name, _, err := splitParts(context.Background(), path, 10, options{output: output}, quiet{})
	if err != nil || name != filepath.Join(output, "Game.xci") {
		t.Fatalf("split: %q %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("the original is kept")
	}
	if _, _, err := splitParts(context.Background(), path, 10, options{output: output}, quiet{}); err == nil {
		t.Fatal("the parts already there are not replaced without -w")
	}
	if _, _, err := splitParts(context.Background(), path, 10, options{output: output, overwrite: true, rmSource: true}, quiet{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the original goes with --rm-source")
	}
	if _, parts, _ := partsSize(name); parts != 3 {
		t.Fatalf("%d parts", parts)
	}
}

// A file FAT32 holds is left as it is.
func TestFat32LeavesSmallFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Small.nsp")
	os.WriteFile(path, []byte("small"), 0o644)
	if name, note, err := fat32(context.Background(), path, options{}, quiet{}); name != "" || note != "it fits in FAT32 as it is" || err != nil {
		t.Fatalf("%q %q %v", name, note, err)
	}
	if _, ok := splitContainer(path); ok {
		t.Fatal("a file is not in parts")
	}
	if _, ok := splitContainer(t.TempDir()); ok {
		t.Fatal("a folder without the name of a game file is not in parts")
	}
}
