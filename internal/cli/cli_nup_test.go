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
	if code != 0 || !strings.Contains(out, "OK Made-up Quest.nup (3 files checked by their SHA-256)") {
		t.Errorf("verify: %d\n%s", code, out)
	}
	// checked before and not changed since
	code, out = run("-V", good)
	if code != 0 || !strings.Contains(out, "not changed since; 3 files checked") {
		t.Errorf("verify again: %d\n%s", code, out)
	}

	code, out = run("-i", good)
	for _, want := range []string{"0400486024DEC000 BASE v65536 Made-up Quest", "publisher: Nobody Games", "0400486024DEC800 UPD v131072",
		"2 files: ", "NUP: ", "OK Made-up Quest.nup (2 titles)"} {
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
	for _, args := range [][]string{{"-C"}, {"-D"}, {"--convert"}, {"--recompress"}, {"--trim"}, {"--split"}} {
		code, out = run(append(args, good)...)
		if code != 0 || !strings.Contains(out, "SKIPPED Made-up Quest.nup (NXPress does not compress, convert or repack NUP files") {
			t.Errorf("%v: %d\n%s", args, code, out)
		}
		if strings.Contains(out, "no file to") {
			t.Errorf("%v: told no file too\n%s", args, out)
		}
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
