package jobs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/nup"
)

// The name a NUP carries for a dump folder without control data comes from the name of the folder,
// without its tags, its version and the word ROM, and with its words set apart.
func TestDumpNameOf(t *testing.T) {
	for folder, want := range map[string]string{
		"MarioTennisFever_v1.1.1":             "Mario Tennis Fever",
		"NintendoSwitchSportsResort_v1.1.0":   "Nintendo Switch Sports Resort",
		"MKW [ROM]":                           "MKW",
		"DKB ROM":                             "DKB",
		"Yoshi and the Mysterious Book":       "Yoshi and the Mysterious Book",
		"Star Fox [0400E88025B1A000][v65536]": "Star Fox",
	} {
		if got := dumpNameOf(folder); got != want {
			t.Errorf("%q: %q, want %q", folder, got, want)
		}
	}
}

// npdmOf is a made-up main.npdm of a program, with its ACI0 where the header says.
func npdmOf(id uint64) []byte {
	data := make([]byte, 0x200)
	copy(data, "META")
	binary.LittleEndian.PutUint32(data[0x70:], 0x100)
	binary.LittleEndian.PutUint32(data[0x74:], 0x40)
	copy(data[0x100:], "ACI0")
	binary.LittleEndian.PutUint64(data[0x110:], id)
	return data
}

// writeDump writes a dump folder without control data: a game with its NPDM and a file.
func writeDump(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	for path, data := range map[string][]byte{
		"base/exefs/main.npdm":     npdmOf(0x0400D4201C6EA000),
		"base/exefs/main":          []byte("main"),
		"base/romfs/Data/game.bin": []byte(strings.Repeat("game data ", 500)),
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// With --rm-source and --rm-dumps a dump folder goes once its NUP is read back and checked; a
// file the NUP does not hold, or one changed since the packing began, stays, with the folders
// that hold it. With --rm-source alone the folder stays whole.
func TestPackNupRemovesDump(t *testing.T) {
	both := Options{RmSource: true, RmDumps: true}
	dir := writeDump(t, "Moonlit Orchard")
	name, note, err := packNup(t.Context(), dir, both, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if _, check, err := nup.Verify(t.Context(), name, 1, 1, nil); err != nil || len(check.Damaged) > 0 {
		t.Fatalf("the NUP left: %v, %v", check.Damaged, err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("the dump folder is still there: %v", err)
	}
	if !strings.HasSuffix(note, dumpDeleted) {
		t.Errorf("the note: %s", note)
	}

	// a file beside the packages of the folder, and one written after the packing began
	dir = writeDump(t, "Harbor Lights")
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("notes of the dump"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "base", "exefs", "main"), later, later); err != nil {
		t.Fatal(err)
	}
	_, note, err = packNup(t.Context(), dir, both, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{"readme.txt", "base/exefs/main"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(kept))); err != nil {
			t.Errorf("%s is gone: %v", kept, err)
		}
	}
	for _, gone := range []string{"base/exefs/main.npdm", "base/romfs"} {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", gone, err)
		}
	}
	if !strings.HasSuffix(note, "2 files stay in the dump folder, which the NUP does not hold as they are: the rest is deleted") {
		t.Errorf("the note: %s", note)
	}

	dir = writeDump(t, "Copper Valley")
	_, note, err = packNup(t.Context(), dir, Options{RmSource: true}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "base", "romfs", "Data", "game.bin")); err != nil {
		t.Errorf("--rm-source alone took the dump folder: %v", err)
	}
	if !strings.HasSuffix(note, keptOriginal) {
		t.Errorf("the note: %s", note)
	}
}

// A dump folder without control data gives its NUP the name of the game its folder tells; the
// hidden file a pack that stopped left is told and kept, not removed by the next one.
func TestPackNupNameAndLeftover(t *testing.T) {
	dir := writeDump(t, "MarioTennisFever_v1.1.1")
	name, _, err := packNup(t.Context(), dir, Options{}, 1, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	container, err := nup.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if got := container.Packages[0].Name(); got != "Mario Tennis Fever" {
		t.Errorf("the name the NUP carries: %q", got)
	}
	if !strings.HasPrefix(filepath.Base(name), "MarioTennisFever_v1.1.1 [0400D4201C6EA000]") {
		t.Errorf("the name of the file stays the one of the folder: %s", filepath.Base(name))
	}

	// a pack that stopped left its hidden file
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	leftover := temporary(name)
	if err := os.WriteFile(leftover, []byte("half a NUP"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := packNup(t.Context(), dir, Options{}, 1, &recording{}); err == nil || !strings.Contains(err.Error(), "left by a work that stopped") {
		t.Errorf("a leftover in the way: %v", err)
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Errorf("the leftover is not this work's to remove: %v", err)
	}
}
