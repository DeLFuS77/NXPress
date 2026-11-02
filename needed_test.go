package main

import (
	"path/filepath"
	"testing"
)

// the free space of a drive is read, also for an output folder not made yet
func TestFreeSpace(t *testing.T) {
	folder := t.TempDir()
	for _, path := range []string{folder, filepath.Join(folder, "not", "made")} {
		if free, err := freeSpace(path); err != nil || free <= 0 {
			t.Errorf("%s: %d, %v", path, free, err)
		}
	}
}

// a file that replaces its original keeps what it grew by, and that adds up: decompressing with
// the originals deleted needs every growth, and beside them the originals of the files made at
// once; one byte less than that worst moment does not fit
func TestSpaceNeededGrowth(t *testing.T) {
	files := []spaceFile{
		{folder: "games", drive: "games", size: 100, most: 150, replaces: true},
		{folder: "games", drive: "games", size: 300, most: 400, replaces: true},
		{folder: "games", drive: "games", size: 200, most: 300, replaces: true},
		// a file that shrinks gives nothing back to the others before it is done
		{folder: "games", drive: "games", size: 50, most: 10, replaces: true},
	}
	// what each one grows by: 50 + 100 + 100; beside its original, 100, 300, 200 and 10
	for multi, need := range map[int]int64{1: 550, 2: 750, 4: 860} {
		short := spaceNeeded(files, multi, func(string) (int64, error) { return need - 1, nil })
		if len(short) != 1 || short[0].Need != need || short[0].Free != need-1 {
			t.Errorf("%d at once: %+v, want need %d", multi, short, need)
		}
		if short := spaceNeeded(files, multi, func(string) (int64, error) { return need, nil }); len(short) != 0 {
			t.Errorf("%d at once: short %+v with %d free", multi, short, need)
		}
	}
}
