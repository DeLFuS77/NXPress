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

// a file in parts weighs all its parts, in a folder or as name.ns0, name.ns1...
func TestSpaceShortParts(t *testing.T) {
	folder := t.TempDir()
	inFolder := filepath.Join(folder, "big.nsp")
	if err := os.Mkdir(inFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{
		filepath.Join(inFolder, "00"): 100, filepath.Join(inFolder, "01"): 50,
		filepath.Join(folder, "c.ns0"): 80, filepath.Join(folder, "c.ns1"): 20,
	} {
		if err := os.WriteFile(name, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{inFolder, filepath.Join(folder, "c.ns0")}
	for _, c := range []struct {
		name    string
		request Request
		need    int64
	}{
		{"made whole in another folder", Request{Mode: "fat32", Files: files, Output: filepath.Join(folder, "out"), Multi: 1}, 250},
		{"made whole in place", Request{Mode: "fat32", Files: files, Multi: 1}, 150},
	} {
		short := spaceShort(c.request, func(string) (int64, error) { return c.need - 1, nil })
		if len(short) != 1 || short[0].Need != c.need {
			t.Errorf("%s: %+v, want need %d", c.name, short, c.need)
		}
	}
}

// two folders of one drive are one drive, also one not made yet: a new file in another folder of
// the drive of its original gives the room of the original back
func TestSpaceOneDrive(t *testing.T) {
	folder := t.TempDir()
	games, output := filepath.Join(folder, "games"), filepath.Join(folder, "out", "not made")
	if err := os.Mkdir(games, 0o755); err != nil {
		t.Fatal(err)
	}
	if volumeOf(games) != volumeOf(output) || volumeOf(games) != volumeOf(folder) {
		t.Errorf("one drive, three names: %s, %s, %s", volumeOf(folder), volumeOf(games), volumeOf(output))
	}
	files := []string{}
	for i, size := range []int{300, 200, 100} {
		path := filepath.Join(games, string(rune('a'+i))+".nsp")
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	request := Request{Mode: "compress", Files: files, Output: output, Multi: 1, RemoveSource: true}
	short := spaceShort(request, func(string) (int64, error) { return 299, nil })
	if len(short) != 1 || short[0].Need != 300 {
		t.Errorf("%+v, want need 300", short)
	}
}
