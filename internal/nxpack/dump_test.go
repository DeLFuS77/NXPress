package nxpack

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// npdm is a main.npdm of a program with its ACI0, and its ACID where a console of the kind has it.
func npdm(program uint64, platform byte) []byte {
	data := make([]byte, 0x600)
	copy(data, "META")
	binary.LittleEndian.PutUint32(data[0x70:], 0x100)
	copy(data[0x100:], "ACI0")
	binary.LittleEndian.PutUint64(data[0x110:], program)
	binary.LittleEndian.PutUint32(data[0x78:], 0x40)
	at := 0x40 + 0x200
	if platform == 2 {
		at = 0x40 + 0x300
	}
	copy(data[at:], "ACID")
	return data
}

func nacpOf(name string, version string) []byte {
	data := make([]byte, 0x4000)
	copy(data, name)
	copy(data[0x200:], "Test Publisher")
	copy(data[0x300*2:], name+" JA")
	copy(data[0x3060:], version)
	return data
}

func writeTree(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for path, data := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDumpToNxAndBack(t *testing.T) {
	const id = 0x0100000000010000
	dump := filepath.Join(t.TempDir(), "Test Game [0100000000010000]")
	icon := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, noise(50, 9)...)
	writeTree(t, dump, map[string][]byte{
		"base/exefs/main.npdm":                  npdm(id, 1),
		"base/exefs/main":                       noise(4000, 1),
		"base/romfs/data/a.bin":                 noise(300<<10, 2),
		"base/romfs/data/same.bin":              noise(9000, 3),
		"base/control/control.nacp":             nacpOf("Test Game", "1.0.0"),
		"base/control/icon_AmericanEnglish.dat": icon,
		"updates/65536/exefs/main.npdm":         npdm(id, 1),
		"updates/65536/exefs/main":              noise(4100, 4),
		"updates/65536/romfs/data/a.bin":        noise(300<<10, 5),
		"updates/65536/control/control.nacp":    nacpOf("Test Game", "1.1.0"),
		"dlc/0100000000011001/0/romfs/dlc.bin":  noise(2000, 6),
	})
	game, _, err := FromDump(dump)
	if err != nil {
		t.Fatal(err)
	}
	if game.ID != id || game.Platform != 1 || len(game.Versions) != 3 {
		t.Fatalf("game: %+v", game)
	}
	update := game.Versions[1]
	paths := map[string]bool{}
	for _, f := range update.Files {
		paths[f.Path] = true
	}
	// the update holds the files of the game it does not replace, and its own control data
	if update.Kind != Update || !paths["romfs/data/same.bin"] || !paths["data/control/control.nacp"] || update.Display != "1.1.0" ||
		update.Names["ja"] != "Test Game JA" {
		t.Fatalf("update: %+v %v", update, paths)
	}
	first := filepath.Join(t.TempDir(), "game.nx")
	if _, err := Write(t.Context(), first, []Game{game}, Options{}, nil); err != nil {
		t.Fatal(err)
	}
	referenceCheck(t, first)
	archive, err := switchfs.ReadNx(first)
	if err != nil {
		t.Fatal(err)
	}
	metadata := archive.Metadata()
	for _, want := range []string{"0100000000010000", "0100000000010800", "0100000000011001"} {
		if metadata[want] == nil {
			t.Errorf("the .nx lacks %s: %v", want, metadata)
		}
	}
	if m := metadata["0100000000010800"]; m == nil || m.Ncap == nil || m.Ncap.DisplayVersion != "1.1.0" {
		t.Errorf("update NACP: %+v", m)
	}

	// back into a folder, and that folder into a .nx again: the same versions, file for file
	back := filepath.Join(t.TempDir(), "back")
	if err := ToDump(t.Context(), first, back, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"base/control/control.nacp", "updates/65536/romfs/data/same.bin", "dlc/0100000000011001/0/base/romfs/dlc.bin"} {
		if _, err := os.Stat(filepath.Join(back, filepath.FromSlash(path))); err != nil {
			t.Errorf("the folder lacks %s", path)
		}
	}
	if _, err := os.Stat(filepath.Join(back, "base", "data")); err == nil {
		t.Error("an empty data folder was written")
	}
	again, _, err := FromDump(back)
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "again.nx")
	if _, err := Write(t.Context(), second, []Game{again}, Options{}, nil); err != nil {
		t.Fatal(err)
	}
	referenceCheck(t, second)
	other, err := switchfs.ReadNx(second)
	if err != nil {
		t.Fatal(err)
	}
	a, b := archive.Applications[0].Snapshots, other.Applications[0].Snapshots
	if len(a) != len(b) {
		t.Fatalf("%d versions, then %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Identity != b[i].Identity {
			t.Errorf("version %d changed on the way back", i)
		}
	}
}

// a .nx packed from an NSP keeps its NCA files: it is not turned into a dump folder
func TestToDumpRefusesNative(t *testing.T) {
	header := make([]byte, 0xC00)
	copy(header[0x200:], "DNCA")
	base := files(t, map[string][]byte{"0123456789abcdef0123456789abcdef.nca": header})
	game := Game{ID: 0x0100000000010000, Platform: 1, Versions: []Snapshot{
		{Kind: Base, Version: 0, Program: 0x0100000000010000, Files: base},
	}}
	nx := filepath.Join(t.TempDir(), "game.nx")
	if _, err := Write(t.Context(), nx, []Game{game}, Options{Workers: 1}, nil); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "dump")
	if err := ToDump(t.Context(), nx, dest, nil); !errors.Is(err, ErrNative) {
		t.Fatalf("ToDump = %v, want ErrNative", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("a folder was left")
	}
}
