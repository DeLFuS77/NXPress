package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/cache"
	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// withoutKeys keeps a test away from any keys and from the cache of the files checked: the
// folders NXPress looks in for a prod.keys are empty folders of the test.
func withoutKeys(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AppData", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	before := cache.Checked
	cache.Checked = &cache.CheckCache{}
	t.Cleanup(func() { cache.Checked = before })
	t.Chdir(t.TempDir())
}

func nupGame() []testnup.Package {
	return []testnup.Package{
		{TitleId: 0x0400486024DEC000, Kind: testnup.Base, Version: 65536,
			Meta: []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Quest")}, {Type: testnup.Publisher, Payload: []byte("Nobody Games")}},
			Files: []testnup.File{{Path: "exefs/main", Data: bytes.Repeat([]byte{1, 2, 3}, 1000)},
				{Path: "romfs/Data/stage.bin", Data: bytes.Repeat([]byte("stage "), 20000), Zstd: true}}},
		{TitleId: 0x0400486024DEC000, Kind: testnup.Update, Version: 131072,
			Files: []testnup.File{{Path: "romfs/Data/stage.bin", Data: bytes.Repeat([]byte("stage 2 "), 20000), Zstd: true}}},
	}
}

// A NUP made of a dump without its control data gives no name, only the label of its dump:
// Rename and Details name it by that label made readable, not by the name of its file.
func TestNupRenameWithoutName(t *testing.T) {
	withoutKeys(t)
	game := nupGame()
	for i := range game {
		game[i].Meta = []testnup.Record{{Type: testnup.SourceDump, Payload: []byte("NintendoSwitchSportsResort_v1.1.0")}}
	}
	folder := t.TempDir()
	path := filepath.Join(folder, "NintendoSwitchSportsResortv1.1.0.nup")
	os.WriteFile(path, testnup.Build(game), 0o644)
	want := "Nintendo Switch Sports Resort [0400486024DEC000][v131072].nup"
	if code, out := run("--rename", "{name} [{id}][v{version}]", path); code != 0 || !strings.Contains(out, "-> "+want) {
		t.Fatalf("rename: %d\n%s", code, out)
	}
	if code, out := run("-i", filepath.Join(folder, want)); code != 0 || strings.Contains(out, "NintendoSwitchSportsResort_v1.1.0 ") {
		t.Errorf("details: %d\n%s", code, out)
	}
}

// --rename names a NUP by what it holds, without keys and with the fields of a game file: the
// name of the game, its title ID, the version of its newest update and its titles (#82).
func TestNupRename(t *testing.T) {
	withoutKeys(t)
	folder := t.TempDir()
	path := filepath.Join(folder, "game.nup")
	os.WriteFile(path, testnup.Build(nupGame()), 0o644)
	const template = "{name} [{id}][v{version}]"
	want := "Made-up Quest [0400486024DEC000][v131072].nup"
	code, out := run("--rename", template, path)
	if code != 0 || !strings.Contains(out, "OK game.nup -> "+want) {
		t.Fatalf("rename: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(folder, want)); err != nil {
		t.Errorf("renamed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the old name is still there: %v", err)
	}
	// renamed again, it keeps its name
	if code, out = run("--rename", template, filepath.Join(folder, want)); code != 0 || !strings.Contains(out, "the name is already that one") {
		t.Errorf("again: %d\n%s", code, out)
	}
	// --keep-named leaves a NUP whose name has its title ID
	if code, out = run("--rename", "{name}", "--keep-named", folder); code != 0 || !strings.Contains(out, "its name has its title ID, it is kept") {
		t.Errorf("--keep-named: %d\n%s", code, out)
	}
	// a folder given, the NUP in it is renamed, and is not told left as it is
	if code, out = run("--rename", "{name} [{id}]", folder); code != 0 || !strings.Contains(out, "Made-up Quest [0400486024DEC000].nup") ||
		strings.Contains(out, "leaves NUP files as they are") {
		t.Errorf("folder: %d\n%s", code, out)
	}
}

// --fix-nup-meta writes again the metadata of a NUP counted from 0, and leaves one counted from 1.
func TestNupFixMeta(t *testing.T) {
	withoutKeys(t)
	packages := nupGame()
	packages[0].Meta = []testnup.Record{{Type: 0, Payload: []byte("META")}, {Type: 4, Payload: []byte("Made-up Quest")}, {Type: 5, Payload: []byte("Nobody Games")}}
	path := filepath.Join(t.TempDir(), "zero.nup")
	os.WriteFile(path, testnup.Build(packages), 0o644)
	if code, out := run("-V", path); code != 0 || !strings.Contains(out, "show its name, version and icon wrong") {
		t.Errorf("verify before: %d\n%s", code, out)
	}
	code, out := run("--fix-nup-meta", path)
	if code != 0 || !strings.Contains(out, "OK zero.nup (its metadata is counted from 1 now, as Eglantine and Lichii read it: 1 package written again") {
		t.Fatalf("fix: %d\n%s", code, out)
	}
	if code, out := run("-V", path); code != 0 || strings.Contains(out, "show its name") || !strings.Contains(out, "3 files checked") {
		t.Errorf("verify after: %d\n%s", code, out)
	}
	if code, out := run("--fix-nup-meta", path); code != 0 || !strings.Contains(out, "SKIPPED zero.nup (its metadata is counted from 1 already") {
		t.Errorf("again: %d\n%s", code, out)
	}
}

func run(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	code := RunCommand(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

// A NUP holds its game decrypted: it is verified and shown without keys, a damaged file of it
// told by its path, and any other work leaves it as it is and says why.
func TestNupWithoutKeys(t *testing.T) {
	withoutKeys(t)
	folder := t.TempDir()
	good := filepath.Join(folder, "Made-up Quest.nup")
	os.WriteFile(good, testnup.Build(nupGame()), 0o644)

	code, out := run("-V", good)
	// and whether Eglantine and Lichii open it and can start its game: its ExeFS lacks main.npdm
	if code != 0 || !strings.Contains(out, "OK Made-up Quest.nup (3 files checked by their SHA-256; Eglantine and Lichii cannot start it: the ExeFS of the game v65536 lacks: main.npdm)") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	// checked before and not changed since
	code, out = run("-V", good)
	if code != 0 || !strings.Contains(out, "not changed since; 3 files checked") || !strings.Contains(out, "Eglantine and Lichii cannot start it") {
		t.Errorf("verify again: %d\n%s", code, out)
	}

	code, out = run("-i", good)
	for _, want := range []string{"0400486024DEC000 BASE v65536 Made-up Quest", "publisher: Nobody Games", "0400486024DEC800 UPD v131072",
		"2 files: ", "NUP: ", "Eglantine and Lichii cannot start it: the ExeFS of the game v65536 lacks: main.npdm", "OK Made-up Quest.nup (2 titles)"} {
		if !strings.Contains(out, want) {
			t.Errorf("info: %q missing (%d)\n%s", want, code, out)
		}
	}

	packages := nupGame()
	damaged := testnup.Build(packages)
	offset, _ := testnup.Extent(damaged, packages, 0, "exefs/main")
	damaged[offset+10] ^= 1
	bad := filepath.Join(folder, "Damaged.nup")
	os.WriteFile(bad, damaged, 0o644)
	code, out = run("-V", bad)
	if code != 1 || !strings.Contains(out, "FAILED Damaged.nup: 1 of 3 files are damaged: game v65536: exefs/main (its SHA-256 does not match)") {
		t.Errorf("damaged: %d\n%s", code, out)
	}

	cut := filepath.Join(folder, "Cut.nup")
	os.WriteFile(cut, damaged[:len(damaged)/2], 0o644)
	if code, out = run("-V", cut); code != 1 || !strings.Contains(out, "cut short") {
		t.Errorf("cut short: %d\n%s", code, out)
	}

	// no other work takes a NUP: it is told, and is no failure
	for _, args := range [][]string{{"-C"}, {"--convert"}, {"--recompress"}, {"--trim"}, {"--split"}} {
		code, out = run(append(args, good)...)
		if code != 0 || !strings.Contains(out, "SKIPPED Made-up Quest.nup (this work leaves NUP files as they are") {
			t.Errorf("%v: %d\n%s", args, code, out)
		}
		if strings.Contains(out, "no file to") {
			t.Errorf("%v: told no file too\n%s", args, out)
		}
	}

	// -D turns a NUP into a folder of the same name, and --nup packs the folder again
	unpacked := t.TempDir()
	copied := filepath.Join(unpacked, "Made-up Quest [0400486024DEC000].nup")
	os.WriteFile(copied, testnup.Build(nupGame()), 0o644)
	if code, out = run("-D", copied); code != 0 || !strings.Contains(out, "OK Made-up Quest [0400486024DEC000].nup -> Made-up Quest [0400486024DEC000] (3 files") {
		t.Errorf("-D: %d\n%s", code, out)
	}
	if data, err := os.ReadFile(filepath.Join(unpacked, "Made-up Quest [0400486024DEC000]", "base", "exefs", "main")); err != nil || len(data) != 3000 {
		t.Errorf("unpacked main: %v %d", err, len(data))
	}
	// a folder there already is not written into
	if code, out = run("-D", copied); code != 0 || !strings.Contains(out, "SKIPPED Made-up Quest [0400486024DEC000].nup (Made-up Quest [0400486024DEC000] is already there") {
		t.Errorf("-D again: %d\n%s", code, out)
	}
	repacked := t.TempDir()
	if code, out = run("--nup", "-o", repacked, filepath.Join(unpacked, "Made-up Quest [0400486024DEC000]")); code != 0 ||
		!strings.Contains(out, "OK Made-up Quest [0400486024DEC000] -> Made-up Quest [0400486024DEC000][v131072].nup (3 files in 2 packages") {
		t.Errorf("--nup: %d\n%s", code, out)
	}
	if code, out = run("-V", "--no-cache", filepath.Join(repacked, "Made-up Quest [0400486024DEC000][v131072].nup")); code != 0 || !strings.Contains(out, "3 files checked") {
		t.Errorf("-V of the new NUP: %d\n%s", code, out)
	}

	// a NUP with an NSP still needs the keys, for the NSP
	nsp := filepath.Join(folder, "game.nsp")
	os.WriteFile(nsp, []byte("PFS0"), 0o644)
	if code, out = run("-V", good, nsp); code != 2 {
		t.Errorf("with an NSP: %d\n%s", code, out)
	}
	// a folder of NUP files alone needs none
	nups := t.TempDir()
	os.WriteFile(filepath.Join(nups, "a.nup"), testnup.Build(nupGame()), 0o644)
	if code, out = run("-V", "--no-cache", nups); code != 0 || !strings.Contains(out, "OK a.nup") {
		t.Errorf("folder: %d\n%s", code, out)
	}
}

// --fix-nup-icon gives a NUP without an icon the image of --nup-icon, and leaves one that has an
// icon; without the versions published turned on in the window, titledb is not looked up.
func TestNupFixIcon(t *testing.T) {
	withoutKeys(t)
	path := filepath.Join(t.TempDir(), "game.nup")
	os.WriteFile(path, testnup.Build(nupGame()), 0o644)
	if code, out := run("--fix-nup-icon", path); code == 0 || !strings.Contains(out, "titledb is not looked up for its icon") {
		t.Errorf("without the setting: %d\n%s", code, out)
	}
	image := filepath.Join(t.TempDir(), "own.webp")
	os.WriteFile(image, testnup.Webp, 0o644)
	if code, out := run("--fix-nup-icon", "--nup-icon", image, path); code != 0 || !strings.Contains(out, "OK game.nup (it carries the image given now, 256×256") {
		t.Fatalf("fix: %d\n%s", code, out)
	}
	if code, out := run("-V", path); code != 0 || !strings.Contains(out, "3 files checked") {
		t.Errorf("verify after: %d\n%s", code, out)
	}
	if code, out := run("--fix-nup-icon", "--nup-icon", image, path); code != 0 || !strings.Contains(out, "SKIPPED game.nup (it carries an icon already") {
		t.Errorf("again: %d\n%s", code, out)
	}
	if code, out := run("--fix-nup-icon", "--nup-icon", image, "--no-nup-icon", path); code != 2 || !strings.Contains(out, "cannot be used together") {
		t.Errorf("both: %d\n%s", code, out)
	}
}
