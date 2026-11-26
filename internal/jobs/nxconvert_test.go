package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/nup"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// A dump folder becomes a .nx; the .nx a NUP; the NUP a .nx again, with the same versions; the
// .nx verifies and unpacks into a folder.
func TestConvertDumpNxAndNup(t *testing.T) {
	dir := writeDump(t, "Quiet Meadow")
	o := Options{}
	first, note, err := packNx(t.Context(), dir, o, 2, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(first) != ".nx" || !strings.Contains(note, "checked whole") {
		t.Fatalf("%s %q", first, note)
	}
	archive, err := switchfs.ReadNx(first)
	if err != nil {
		t.Fatal(err)
	}
	if archive.Metadata()["0400d4201c6ea000"] == nil {
		t.Fatalf("the game is not in the .nx: %v", archive.Metadata())
	}

	if _, note, err := verifyNx(t.Context(), first, &recording{}); err != nil || !strings.Contains(note, "1 version checked") {
		t.Fatalf("verify: %q %v", note, err)
	}

	// into a NUP, through a folder that goes after
	nupFile, _, err := nupFromNx(t.Context(), first, o, 2, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	if _, check, err := nup.Verify(t.Context(), nupFile, 1, 1, nil); err != nil || len(check.Damaged) > 0 {
		t.Fatalf("the NUP: %v %v", check.Damaged, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(first), "*folder*"))
	if len(leftovers) > 0 {
		t.Errorf("folders left: %v", leftovers)
	}

	// the NUP into a .nx again, beside a copy of the first one
	moved := filepath.Join(t.TempDir(), filepath.Base(nupFile))
	if err := os.Rename(nupFile, moved); err != nil {
		t.Fatal(err)
	}
	second, _, err := packNx(t.Context(), moved, o, 2, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := switchfs.ReadNx(second)
	if err != nil {
		t.Fatal(err)
	}
	if a, b := archive.Applications[0].Snapshots[0].Identity, again.Applications[0].Snapshots[0].Identity; a != b {
		t.Error("the game changed on its way through the NUP")
	}

	// unpacked: the folder holds the game as the dump did
	folder, _, err := unpackNx(t.Context(), second, o, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(folder, "base", "romfs", "Data", "game.bin"))
	if err != nil || string(data) != strings.Repeat("game data ", 500) {
		t.Errorf("the folder: %v", err)
	}
}

// --to-nx is an action of its own, which takes dump folders and NUP files, and needs no keys.
func TestToNxOption(t *testing.T) {
	o := Options{ToNx: true, Files: []string{writeDump(t, "Amber Coast")}}
	if err := o.Check(); err != nil {
		t.Fatal(err)
	}
	if o.Action() != "tonx" || NeedsKeys(o) {
		t.Errorf("action %q, keys %v", o.Action(), NeedsKeys(o))
	}
	both := Options{ToNx: true, Nup: true}
	if err := both.Check(); err == nil {
		t.Error("--to-nx and --nup together")
	}
}

// Combine with the format for emulators chosen in the window: with .nx the dump folder and the NUP
// become .nx files and a .nx is left; with NUP the .nx becomes a NUP and a NUP is left.
func TestCombineFormatForEmulators(t *testing.T) {
	dir := writeDump(t, "Quiet Meadow")
	nx, _, err := packNx(t.Context(), writeDump(t, "Amber Coast"), Options{}, 2, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	nupFile, _, err := packNup(t.Context(), writeDump(t, "Silver Pines"), Options{}, 2, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	out := &recording{}
	if failed := RunJobs(t.Context(), Options{PackGames: true, PackNx: true, Files: []string{dir, nupFile, nx}, Multi: 1, Threads: 2}, out); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if found, _ := filepath.Glob(filepath.Join(filepath.Dir(dir), "*.nx")); len(found) == 0 {
		t.Error("the dump folder did not become a .nx")
	}
	// its name has brackets, which Glob would read as a pattern
	if _, err := os.Stat(strings.TrimSuffix(nupFile, ".nup") + ".nx"); err != nil {
		t.Errorf("the NUP did not become a .nx: %v", err)
	}
	if len(out.skipped) != 1 || !strings.Contains(out.skipped[0], "a .nx already") {
		t.Errorf("skipped: %v", out.skipped)
	}

	out = &recording{}
	if failed := RunJobs(t.Context(), Options{PackGames: true, Files: []string{nx, nupFile}, Multi: 1, Threads: 2}, out); failed != 0 {
		t.Fatalf("%d failed", failed)
	}
	if found, _ := filepath.Glob(filepath.Join(filepath.Dir(nx), "*.nup")); len(found) == 0 {
		t.Error("the .nx did not become a NUP")
	}
	if len(out.skipped) != 1 || !strings.Contains(out.skipped[0], "a NUP already") {
		t.Errorf("skipped: %v", out.skipped)
	}
}
