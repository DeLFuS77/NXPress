package main

import (
	"os"
	"path/filepath"
	"testing"
)

// the space a work needs: every new file kept, or of the files that replace their originals only
// the ones made at once
func TestSpaceShort(t *testing.T) {
	folder := t.TempDir()
	files := []string{}
	for i, size := range []int{300, 200, 100} {
		path := filepath.Join(folder, string(rune('a'+i))+".nsp")
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	free := func(left int64) func(string) (int64, error) {
		return func(string) (int64, error) { return left, nil }
	}
	for _, c := range []struct {
		name    string
		request Request
		left    int64
		need    int64
	}{
		{"every file kept", Request{Mode: "compress", Files: files, Multi: 1}, 599, 600},
		{"fits", Request{Mode: "compress", Files: files, Multi: 1}, 600, 0},
		{"originals removed, one at once", Request{Mode: "compress", Files: files, Multi: 1, RemoveSource: true}, 299, 300},
		{"originals removed, two at once", Request{Mode: "compress", Files: files, Multi: 2, RemoveSource: true}, 499, 500},
		{"recompress in place", Request{Mode: "recompress", Files: files, Multi: 1}, 300, 0},
		{"verify writes nothing", Request{Mode: "verify", Files: files, Multi: 1}, 0, 0},
	} {
		short := spaceShort(c.request, free(c.left))
		switch {
		case c.need == 0 && len(short) != 0:
			t.Errorf("%s: short %+v", c.name, short)
		case c.need > 0 && (len(short) != 1 || short[0].Need != c.need || short[0].Free != c.left):
			t.Errorf("%s: %+v, want need %d", c.name, short, c.need)
		}
	}
}

// the free space of a drive is read, also for an output folder not made yet
func TestFreeSpace(t *testing.T) {
	folder := t.TempDir()
	for _, path := range []string{folder, filepath.Join(folder, "not", "made")} {
		if free, err := freeSpace(path); err != nil || free <= 0 {
			t.Errorf("%s: %d, %v", path, free, err)
		}
	}
}
