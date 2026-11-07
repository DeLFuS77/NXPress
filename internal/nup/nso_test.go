package nup

import (
	"bytes"
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// madeUpNso is a module of three segments, LZ4 when compressed is set, with their SHA-256.
func madeUpNso(compressed bool) []byte {
	return testnup.Nso(compressed,
		append(bytes.Repeat([]byte("text of the module, made up "), 900), noise(4000, 8)...),
		append(bytes.Repeat([]byte("read-only data "), 300), noise(700, 9)...),
		bytes.Repeat([]byte{0, 0, 0, 1}, 2000))
}

// checkNso writes a module into a check in pieces of the size given, as the reader of a NUP
// hands them, and gives what the check tells.
func checkNso(module []byte, size int64, piece int) string {
	check := newNsoCheck(size)
	for at := 0; at < len(module); at += piece {
		check.Write(module[at:min(at+piece, len(module))])
	}
	return check.result()
}

func TestNsoCheckGoodModules(t *testing.T) {
	for _, compressed := range []bool{true, false} {
		module := madeUpNso(compressed)
		for _, piece := range []int{1, 7, 0x100, 4096, len(module)} {
			if problem := checkNso(module, int64(len(module)), piece); problem != "" {
				t.Errorf("compressed %v, pieces of %d: %s", compressed, piece, problem)
			}
		}
	}
	// the LZ4 of the tests is LZ4: it holds matches, and decompresses back
	data := bytes.Repeat([]byte("abcdefgh"), 1000)
	block := testnup.Lz4(data)
	if out, err := lz4Decompress(block, len(data)); err != nil || !bytes.Equal(out, data) || len(block) > len(data)/10 {
		t.Fatalf("LZ4 of %d bytes in %d: %v", len(data), len(block), err)
	}
}

func TestNsoCheckTellsAModuleCutShort(t *testing.T) {
	module := madeUpNso(true)
	// bytes lost inside the text segment, as in a dump that lost them: the rest comes earlier
	cut := append(append([]byte{}, module[:0x300]...), module[0x300+980:]...)
	if problem := checkNso(cut, int64(len(cut)), 4096); problem != "it is 980 bytes shorter than its header says" {
		t.Errorf("problem %q", problem)
	}
	if problem := checkNso(module[:len(module)-31], int64(len(module)-31), 512); problem != "it is 31 bytes shorter than its header says" {
		t.Errorf("cut at the end: %q", problem)
	}
	if problem := checkNso(module[:0x80], 0x80, 512); problem != "it ends in its header (128 of 256 bytes)" {
		t.Errorf("cut in the header: %q", problem)
	}
}

func TestNsoCheckTellsADamagedSegment(t *testing.T) {
	module := madeUpNso(true)
	le := binary.LittleEndian
	// a hash of the header that does not match
	wrongHash := append([]byte{}, module...)
	wrongHash[0xC0] ^= 0xFF
	if problem := checkNso(wrongHash, int64(len(wrongHash)), 1000); problem != "the SHA-256 of its ro segment does not match its header" {
		t.Errorf("wrong hash: %q", problem)
	}
	// a byte of the literals of the data segment changed: it decompresses, not to what was hashed
	damaged := append([]byte{}, module...)
	data := int(le.Uint32(module[0x30:]))
	damaged[data+1] ^= 0x01
	if problem := checkNso(damaged, int64(len(damaged)), 1000); problem != "the SHA-256 of its data segment does not match its header" &&
		!strings.HasPrefix(problem, "its data segment does not decompress") {
		t.Errorf("damaged data: %q", problem)
	}
	// an uncompressed module checked by its hashes alone
	plain := madeUpNso(false)
	plain[0x100+5] ^= 0x01
	if problem := checkNso(plain, int64(len(plain)), 1000); problem != "the SHA-256 of its text segment does not match its header" {
		t.Errorf("plain: %q", problem)
	}
	// LZ4 that runs past what the header says
	broken := append([]byte{}, module...)
	le.PutUint32(broken[0x18:], le.Uint32(broken[0x18:])-10)
	if problem := checkNso(broken, int64(len(broken)), 1000); problem != "its text segment does not decompress to the 29190 bytes its header says (LZ4)" {
		t.Errorf("broken: %q", problem)
	}
	// a size no LZ4 data of its length can decompress to
	huge := append([]byte{}, module...)
	le.PutUint32(huge[0x38:], 1<<29)
	if problem := checkNso(huge, int64(len(huge)), 1000); problem != "its data segment cannot decompress to the 536870912 bytes its header says" {
		t.Errorf("huge: %q", problem)
	}
}

func TestNsoCheckLeavesOtherFilesAlone(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("NSO"), noise(3000, 1), []byte("NPDM made up"), nso(1)} {
		if problem := checkNso(data, int64(len(data)), 100); problem != "" {
			t.Errorf("%q: %s", data[:min(8, len(data))], problem)
		}
	}
	// segments compressed in another way are checked by the size only
	other := madeUpNso(true)
	other[0x0C] |= nsoFlagOther
	other[0x100+3] ^= 0xFF
	if problem := checkNso(other, int64(len(other)), 1000); problem != "" {
		t.Errorf("other compression: %s", problem)
	}
	if problem := checkNso(other[:len(other)-1], int64(len(other)-1), 1000); problem != "it is 1 bytes shorter than its header says" {
		t.Errorf("other compression cut short: %q", problem)
	}
}

func TestIsCodeModule(t *testing.T) {
	for path, want := range map[string]bool{
		"exefs/main": true, "exefs/rtld": true, "exefs/sdk": true, "exefs/subsdk0": true, "exefs/subsdk9": true,
		"ExeFS/MAIN": true, "program/exefs/main": true, `base\exefs\main`: true, "exefs/main.npdm": false, "romfs/main": false,
		"main": false, "exefs/subsdk": false, "exefs/subsdk10": false, "exefs/maindata": false,
	} {
		if IsCodeModule(path) != want {
			t.Errorf("%s: %v", path, !want)
		}
	}
}

// withCode is a game and its update, each with its code modules, the main of the update the one
// given.
func withCode(main []byte) []testnup.Package {
	good := madeUpNso(true)
	return []testnup.Package{
		{TitleId: 0x0400A3A01EE4C000, Kind: testnup.Base, Version: 0,
			Files: []testnup.File{
				{Path: "exefs/main", Data: good, Zstd: true},
				{Path: "exefs/rtld", Data: good},
				{Path: "exefs/main.npdm", Data: []byte("NPDM made up")},
				{Path: "romfs/Data/a.bin", Data: noise(5000, 3), Zstd: true},
			}},
		{TitleId: 0x0400A3A01EE4C000, Kind: testnup.Update, Version: 655360,
			Files: []testnup.File{
				{Path: "exefs/main", Data: main, Zstd: true},
				{Path: "exefs/sdk", Data: good, Zstd: true},
				{Path: "exefs/subsdk0", Data: madeUpNso(false)},
			}},
	}
}

func TestVerifyChecksTheCode(t *testing.T) {
	good := madeUpNso(true)
	_, check, err := Verify(context.Background(), write(t, testnup.BuildWith(withCode(good), testnup.Options{Dedup: true})), 3, 1, nil)
	if err != nil || len(check.Damaged) != 0 || check.Modules != 5 {
		t.Fatalf("good code: %+v %v", check, err)
	}

	// a dump that lost bytes of its main: the NUP holds it as it was packed, its hashes match
	cut := append(append([]byte{}, good[:0x400]...), good[0x400+980:]...)
	for _, workers := range []int{1, 4} {
		_, check, err := Verify(context.Background(), write(t, testnup.Build(withCode(cut))), workers, 2, nil)
		if err != nil || len(check.Damaged) != 1 {
			t.Fatalf("cut main: %+v %v", check, err)
		}
		if d := check.Damaged[0]; d.Package != "update v655360" || d.Path != "exefs/main" || !d.Code || d.Reason != "it is 980 bytes shorter than its header says" {
			t.Errorf("damaged %+v", d)
		}
	}

	// a module whose hash in its header does not match, besides a file damaged in the NUP
	wrong := append([]byte{}, good...)
	wrong[0xA0] ^= 0xFF
	packages := withCode(wrong)
	data := testnup.Build(packages)
	damage(data, packages, 0, "romfs/Data/a.bin", 10)
	_, check, err = Verify(context.Background(), write(t, data), 2, 1, nil)
	found := map[string]Damaged{}
	for _, d := range check.Damaged {
		found[d.Package+"/"+d.Path] = d
	}
	if err != nil || len(found) != 2 || found["update v655360/exefs/main"].Reason != "the SHA-256 of its text segment does not match its header" ||
		!found["update v655360/exefs/main"].Code || found["game v0/romfs/Data/a.bin"].Code {
		t.Errorf("damaged %+v %v", check.Damaged, err)
	}

	// a module damaged in the NUP is damaged by its data, not by the dump
	packages = withCode(good)
	data = testnup.Build(packages)
	damage(data, packages, 1, "exefs/subsdk0", 0x300)
	_, check, err = Verify(context.Background(), write(t, data), 2, 1, nil)
	if err != nil || len(check.Damaged) != 1 || check.Damaged[0].Code || check.Damaged[0].Reason != "its SHA-256 does not match" {
		t.Errorf("damaged in the NUP: %+v %v", check.Damaged, err)
	}
}

func TestCheckDumpCode(t *testing.T) {
	good := madeUpNso(true)
	cut := append(append([]byte{}, good[:0x400]...), good[0x400+989:]...)
	root := filepath.Join(t.TempDir(), "Made-up Island")
	writeTree(t, root, map[string][]byte{
		"base/exefs/main":                    cut,
		"base/exefs/rtld":                    good,
		"base/exefs/sdk":                     good[:len(good)-297],
		"base/exefs/main.npdm":               npdm(0x0400A3A01EE4C000),
		"base/romfs/Data/a.bin":              noise(500, 1),
		"updates/655360/exefs/main":          good,
		"updates/655360/exefs/subsdk0":       madeUpNso(false),
		"updates/655360/exefs/main.npdm":     npdm(0x0400A3A01EE4C000),
		"dlc/0400a3a01ee4d001/0/romfs/a.bin": noise(100, 2),
	})
	check, err := CheckDumpCode(context.Background(), root)
	if err != nil || check.Modules != 5 || len(check.Damaged) != 2 {
		t.Fatalf("%+v %v", check, err)
	}
	if d := check.Damaged[0]; d.Package != "game v0" || d.Path != "exefs/main" || !d.Code || d.Reason != "it is 989 bytes shorter than its header says" {
		t.Errorf("main %+v", d)
	}
	if d := check.Damaged[1]; d.Path != "exefs/sdk" || d.Reason != "it is 297 bytes shorter than its header says" {
		t.Errorf("sdk %+v", d)
	}

	// the new layout of OunceCake, and files that are not modules
	root = filepath.Join(t.TempDir(), "Made-up Quest")
	writeTree(t, root, map[string][]byte{
		"base/program/0/exefs/main":      good,
		"base/program/0/exefs/main.npdm": npdm(0x0400486024DEC000),
		"base/program/0/exefs/subsdk1":   []byte("not a module"),
		"base/program/0/romfs/a.bin":     noise(100, 3),
	})
	if check, err := CheckDumpCode(context.Background(), root); err != nil || check.Modules != 1 || len(check.Damaged) != 0 {
		t.Errorf("new layout: %+v %v", check, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckDumpCode(ctx, root); err == nil {
		t.Error("a cancelled check goes on")
	}
}
