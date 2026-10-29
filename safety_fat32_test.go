package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// safetyHolder opens a file, as an antivirus or an indexer would, once the check of the new
// copy starts.
type safetyHolder struct {
	quiet
	path string
	held *os.File
}

func (h *safetyHolder) progress(path string, step string) func(int64, int64) {
	return func(int64, int64) {
		if step == "Verifying" && h.held == nil {
			h.held, _ = os.Open(h.path)
		}
	}
}

func (h *safetyHolder) release() {
	if h.held != nil {
		h.held.Close()
	}
}

// Split in place while another program holds a part open: the original is deleted only once
// its parts have its name, and is back in its place when they cannot have it.
func TestFat32SplitInPlaceKeepsTheOriginalUntilTheEnd(t *testing.T) {
	defer func(size int64) { fat32Part = size }(fat32Part)
	fat32Part = 4
	folder := t.TempDir()
	path := filepath.Join(folder, "Game.nsp")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	out := &safetyHolder{path: filepath.Join(temporary(path), "01")}
	_, _, err := splitParts(context.Background(), path, 10, options{}, out)
	out.release()
	info, statErr := os.Stat(path)
	switch {
	case statErr != nil:
		t.Fatalf("the original is gone and the split gave %v; left: %v", err, safetyList(folder))
	case err != nil && info.IsDir():
		t.Fatalf("the split failed, %v, but the parts have the name", err)
	case err != nil:
		if got, _ := os.ReadFile(path); string(got) != "0123456789" {
			t.Fatalf("the original holds %q", got)
		}
	case !info.IsDir():
		t.Fatal("the split went well and the original is still a file")
	}
	// the parts held open could not be removed with the failed split; nothing else is left
	for _, name := range safetyList(folder) {
		if name != "Game.nsp" && name != ".Game.nsp.nxpress" {
			t.Fatalf("left: %v", safetyList(folder))
		}
	}
}

// Join a folder of parts in place while another program holds a part open: the parts go only
// once the whole file has their name, and stay when it cannot have it.
func TestFat32JoinInPlaceKeepsThePartsUntilTheEnd(t *testing.T) {
	folder := t.TempDir()
	parts := filepath.Join(folder, "Game.nsp")
	os.Mkdir(parts, 0o755)
	os.WriteFile(filepath.Join(parts, "00"), []byte("0123456789"), 0o644)
	os.WriteFile(filepath.Join(parts, "01"), []byte("abcdefghij"), 0o644)
	os.WriteFile(filepath.Join(parts, "02"), []byte("KLM"), 0o644)
	out := &safetyHolder{path: filepath.Join(parts, "02")}
	_, _, err := joinParts(context.Background(), parts, parts, options{}, out)
	out.release()
	info, statErr := os.Stat(parts)
	switch {
	case statErr != nil:
		t.Fatalf("the parts are gone and the join gave %v; left: %v", err, safetyList(folder))
	case err != nil && !reflect.DeepEqual(safetyList(parts), []string{"00", "01", "02"}):
		t.Fatalf("the join failed, %v, and the parts are %v", err, safetyList(parts))
	case err == nil && info.IsDir():
		t.Fatal("the join went well and the parts are still there")
	case err == nil:
		if got, _ := os.ReadFile(parts); string(got) != "0123456789abcdefghijKLM" {
			t.Fatalf("joined: %q", got)
		}
	}
	if left := safetyList(folder); len(left) != 1 {
		t.Fatalf("left: %v", left)
	}
}

// Parts name.ns0, name.ns1... are deleted only once the whole file has its name: one held open
// leaves the whole file there.
func TestFat32JoinNamesTheFileBeforeThePartsGo(t *testing.T) {
	folder := t.TempDir()
	for i, part := range []string{"0123456789", "abcdefghij", "KLM"} {
		os.WriteFile(filepath.Join(folder, "Game.ns"+string(rune('0'+i))), []byte(part), 0o644)
	}
	out := &safetyHolder{path: filepath.Join(folder, "Game.ns2")}
	name, _, err := joinParts(context.Background(), filepath.Join(folder, "Game.ns0"), filepath.Join(folder, "Game.nsp"), options{}, out)
	out.release()
	if got, readErr := os.ReadFile(filepath.Join(folder, "Game.nsp")); readErr != nil || string(got) != "0123456789abcdefghijKLM" {
		t.Fatalf("the whole file: %q %v; the join gave %q %v; left: %v", got, readErr, name, err, safetyList(folder))
	}
	if err != nil && !strings.Contains(err.Error(), "could not be deleted") {
		t.Fatalf("the join: %v", err)
	}
}

// place moves what has the name aside and deletes it once the new one has the name; when the
// new one cannot have it, the one there is back.
func TestPlaceDeletesTheOriginalLast(t *testing.T) {
	folder := t.TempDir()
	target := filepath.Join(folder, "Game.nsp")
	written := filepath.Join(folder, ".Game.nsp.nxpress")
	os.WriteFile(target, []byte("original"), 0o644)
	if err := place(filepath.Join(folder, "missing"), target); err == nil {
		t.Fatal("placed a file that is not there")
	}
	if got, _ := os.ReadFile(target); string(got) != "original" || len(safetyList(folder)) != 1 {
		t.Fatalf("after a failed place: %q, %v", got, safetyList(folder))
	}
	os.WriteFile(written, []byte("new"), 0o644)
	if err := place(written, target); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" || len(safetyList(folder)) != 1 {
		t.Fatalf("after a place: %q, %v", got, safetyList(folder))
	}
}

// finish with -w deletes nothing before the rename: when it fails, the file there stays.
func TestFinishDeletesNothingFirst(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "Game.nsz")
	os.WriteFile(path, []byte("there"), 0o644)
	if err := finish(options{overwrite: true}, filepath.Join(folder, "missing"), path); err == nil {
		t.Fatal("finished a file that is not there")
	}
	if got, _ := os.ReadFile(path); string(got) != "there" {
		t.Fatalf("the file there: %q", got)
	}
	written := temporary(path)
	os.WriteFile(written, []byte("new"), 0o644)
	if err := finish(options{}, written, path); err == nil || !safetyExists(written) {
		t.Fatalf("without -w: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "there" {
		t.Fatalf("without -w the file there holds %q", got)
	}
}
