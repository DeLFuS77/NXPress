package nup

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// npdm is a made-up main.npdm of a program, with its ACI0 where the header says.
func npdm(id uint64) []byte {
	data := make([]byte, 0x200)
	copy(data, "META")
	binary.LittleEndian.PutUint32(data[0x70:], 0x100)
	binary.LittleEndian.PutUint32(data[0x74:], 0x40)
	copy(data[0x100:], "ACI0")
	binary.LittleEndian.PutUint64(data[0x110:], id)
	return data
}

// nso is a made-up main with the build ID in its header.
func nso(build byte) []byte {
	data := make([]byte, 0x200)
	copy(data, "NSO0")
	for i := 0x40; i < 0x60; i++ {
		data[i] = build
	}
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

// oldDump is a dump in the layout the packer of the format reads: base, updates, dlc, with the
// listings an FTP client leaves, a file the update shares with the game, an empty file, big
// files zstd makes smaller and one it does not.
func oldDump(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "Made-up Dump")
	shared := bytes.Repeat([]byte("shared asset "), 50000)
	writeTree(t, root, map[string][]byte{
		".listing":                             []byte("dr-xr-xr-x 1 ftp ftp 0 Jan 01  1970 base\n"),
		"base/exefs/main":                      nso(1),
		"base/exefs/main.npdm":                 npdm(0x0400486024DEC000),
		"base/exefs/.listing":                  []byte("listing"),
		"base/romfs/Data/stage.bin":            bytes.Repeat([]byte("stage data "), 3000),
		"base/romfs/Data/shared.bin":           shared,
		"base/romfs/Data/big.bin":              bytes.Repeat([]byte("0123456789abcdef"), 700_000),
		"base/romfs/Data/noise.bin":            noise(9<<20, 7),
		"base/romfs/empty.txt":                 nil,
		"base/romfs/Sound/Wave 01 ñ.bwav":      noise(5000, 8),
		"updates/65536/exefs/main":             nso(2),
		"updates/65536/exefs/main.npdm":        npdm(0x0400486024DEC000),
		"updates/65536/romfs/Data/shared.bin":  shared,
		"updates/65536/romfs/Data/patch.bin":   []byte("patched"),
		"dlc/0400486024ded001/0/romfs/dlc.bin": noise(3000, 9),
	})
	if err := os.MkdirAll(filepath.Join(root, "base", "romfs", "Empty Folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func pack(t *testing.T, src string, o PackOptions) (string, PackResult) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "game.nup")
	result, err := Pack(context.Background(), src, out, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out, result
}

func TestPackOldLayout(t *testing.T) {
	src := oldDump(t)
	for _, level := range []int{3, 0} {
		calls, last := 0, int64(0)
		out := filepath.Join(t.TempDir(), "game.nup")
		result, err := Pack(context.Background(), src, out, PackOptions{Level: level, Workers: 3, Name: "Made-up Quest"}, func(done int64, total int64) {
			calls++
			if done < last || done > total {
				t.Errorf("progress %d after %d, of %d", done, last, total)
			}
			last = done
		})
		if err != nil {
			t.Fatal(err)
		}
		if calls < 2 || last != result.Size || result.Files != 13 || result.Shared != 2 || len(result.Notes) != 0 {
			t.Errorf("level %d: %d files, %d shared, %d notes, %d calls, last %d of %d", level, result.Files, result.Shared, len(result.Notes), calls, last, result.Size)
		}
		if level == 3 && result.Zstd == 0 || level == 0 && result.Zstd != 0 {
			t.Errorf("level %d: %d files with zstd", level, result.Zstd)
		}
		_, check, err := Verify(context.Background(), out, 2, 2, nil)
		if err != nil || len(check.Damaged) != 0 || check.Files != 13 || check.Unplaced != 0 {
			t.Fatalf("level %d: %v %+v", level, err, check)
		}
		container, err := Open(out)
		if err != nil {
			t.Fatal(err)
		}
		kinds := []string{}
		for _, pkg := range container.Packages {
			kinds = append(kinds, Label(pkg)+" "+pkg.TitleIDText())
		}
		if strings.Join(kinds, ", ") != "game v0 0400486024DEC000, update v65536 0400486024DEC800, DLC 0400486024DED001 v0 0400486024DED001" {
			t.Errorf("packages %v", kinds)
		}
		game := container.Packages[0]
		if game.Name() != "Made-up Quest" || !bytes.Equal(game.Meta[0x0007], nso(1)[0x40:0x60]) || game.ExefsDir == NoDir ||
			string(game.Meta[0x0008]) != "Made-up Dump" || container.Packages[2].ExefsDir != NoDir || container.Packages[2].RomfsDir == NoDir {
			t.Errorf("game %+v", game)
		}
		if container.Packages[1].Name() != "" || container.Packages[1].Flags != 0 {
			t.Errorf("update %+v", container.Packages[1])
		}
		// the listings are left out, the empty folder kept
		names := []string{}
		container.Walk(0, func(path string, _ int) error { names = append(names, path); return nil })
		if strings.Contains(strings.Join(names, " "), ".listing") {
			t.Errorf("listing packed: %v", names)
		}
		// the big file zstd makes smaller is stored with it, the noise as it is
		for _, f := range container.Files {
			if f.Name == "big.bin" && level == 3 && f.Codec != CodecZstd || f.Name == "noise.bin" && f.Codec != CodecRaw {
				t.Errorf("%s stored with codec %d", f.Name, f.Codec)
			}
		}

		// unpacked, it is the folder again, but its listings
		dest := filepath.Join(t.TempDir(), "unpacked")
		unpacked, err := Unpack(context.Background(), out, dest, 3, 3, nil)
		if err != nil {
			t.Fatal(err)
		}
		if unpacked.Files != 13 || unpacked.Size != result.Size {
			t.Errorf("unpacked %+v", unpacked)
		}
		compareTrees(t, src, dest)
	}
}

// compareTrees checks that two folders hold the same files and folders, but the listings.
func compareTrees(t *testing.T, want string, got string) {
	t.Helper()
	list := func(root string) map[string]string {
		result := map[string]string{}
		filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || path == root || entry.Name() == ".listing" {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				result[filepath.ToSlash(rel)] = "folder"
			} else {
				data, _ := os.ReadFile(path)
				result[filepath.ToSlash(rel)] = string(data)
			}
			return nil
		})
		return result
	}
	a, b := list(want), list(got)
	for path, data := range a {
		if b[path] != data {
			t.Errorf("%s differs after unpacking (%d bytes, %d)", path, len(data), len(b[path]))
		}
	}
	for path := range b {
		if _, ok := a[path]; !ok {
			t.Errorf("%s is new after unpacking", path)
		}
	}
}

// The new layout of OunceCake: program/<index> with exefs, romfs and control, which gives the
// names, the publisher, the version and the icon of the game.
func TestPackNewLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "dump")
	nacp := make([]byte, 0x4000)
	copy(nacp[0:], "Made-up Quest")
	copy(nacp[0x200:], "Nobody Games")
	copy(nacp[2*0x300:], "メイドアップ")
	copy(nacp[0x3060:], "1.2.0")
	writeTree(t, root, map[string][]byte{
		"base/program/0/exefs/main":                                  nso(1),
		"base/program/0/exefs/main.npdm":                             npdm(0x040074A01BF12000),
		"base/program/0/romfs/a.bin":                                 []byte("a"),
		"base/program/0/control/control.nacp":                        nacp,
		"base/program/0/control/icon_AmericanEnglish.dat":            testnup.Jpeg,
		"base/program/0/meta/x.cnmt":                                 []byte("cnmt"),
		"updates/131072/program/0/exefs/main.npdm":                   npdm(0x040074A01BF12000),
		"updates/131072/program/0/romfs/a.bin":                       []byte("b"),
		"dlc/040074a01bf13001/0/base/program/0/romfs/d.bin":          []byte("dlc"),
		"dlc/040074a01bf13001/0/updates/65536/program/0/romfs/d.bin": []byte("dlc update"),
	})
	out, result := pack(t, root, PackOptions{Level: 3, Workers: 2})
	if len(result.Notes) != 1 || !strings.Contains(result.Notes[0], "no updates of DLC") {
		t.Errorf("notes %v", result.Notes)
	}
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	game := container.Packages[0]
	if len(container.Packages) != 3 || game.Name() != "Made-up Quest" || game.Publisher() != "Nobody Games" ||
		game.DisplayVersion() != "1.2.0" || !bytes.Equal(container.Icon(), testnup.Jpeg) || game.TitleID != 0x040074A01BF12000 {
		t.Fatalf("%+v", container.Packages)
	}
	if localized, _ := parseLocalized(game.Meta[tlvLocalizedTitles]); string(localized["ja"]) != "メイドアップ" {
		t.Errorf("localized %q", localized)
	}
	if update := container.Packages[1]; update.Kind != KindUpdate || update.Version != 131072 || update.ExefsDir == NoDir {
		t.Errorf("update %+v", update)
	}
	if dlc := container.Packages[2]; dlc.Kind != KindAddon || dlc.TitleID != 0x040074A01BF13001 || dlc.RomfsDir == NoDir {
		t.Errorf("DLC %+v", dlc)
	}
	if _, check, err := Verify(context.Background(), out, 1, 1, nil); err != nil || len(check.Damaged) != 0 {
		t.Errorf("%v %+v", err, check)
	}
}

func TestPackOtherLayouts(t *testing.T) {
	// the exefs and romfs of one package, at the root
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{"exefs/main.npdm": npdm(0x0100000000010000), "romfs/a.bin": []byte("a")})
	out, _ := pack(t, root, PackOptions{Level: 3})
	if container, err := Open(out); err != nil || container.Packages[0].Kind != KindBase || container.Packages[0].TitleID != 0x0100000000010000 {
		t.Errorf("%v %+v", err, container)
	}
	// updates only: each is a standalone update
	root = t.TempDir()
	writeTree(t, root, map[string][]byte{"updates/65536/romfs/a.bin": []byte("a"), "updates/131072/romfs/a.bin": []byte("b")})
	out, _ = pack(t, root, PackOptions{})
	container, err := Open(out)
	if err != nil || len(container.Packages) != 2 || container.Packages[0].Flags != 1 || container.Packages[0].Version != 65536 {
		t.Errorf("%v %+v", err, container.Packages)
	}
	// nothing to pack, several programs, an output already there
	for name, files := range map[string]map[string][]byte{
		"nothing":  {"base/exefs/.listing": []byte("x"), "readme.txt": []byte("x")},
		"programs": {"base/program/0/romfs/a": []byte("a"), "base/program/1/romfs/b": []byte("b")},
	} {
		root := t.TempDir()
		writeTree(t, root, files)
		if _, err := Pack(context.Background(), root, filepath.Join(t.TempDir(), "x.nup"), PackOptions{}, nil); err == nil {
			t.Errorf("%s: packed", name)
		}
	}
	there := filepath.Join(t.TempDir(), "there.nup")
	os.WriteFile(there, []byte("x"), 0o644)
	if _, err := Pack(context.Background(), oldDump(t), there, PackOptions{}, nil); !errors.Is(err, fs.ErrExist) {
		t.Errorf("over a file: %v", err)
	}
	// a library with a folder of updates is no dump
	library := t.TempDir()
	writeTree(t, library, map[string][]byte{"updates/Game [0100000000010800][v65536].nsp": []byte("PFS0"), "base/Game.nsp": []byte("PFS0")})
	if IsDump(t.TempDir()) || IsDump(library) || !IsDump(oldDump(t)) || !IsDump(root) {
		t.Error("IsDump")
	}
}

func TestPackStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := Pack(ctx, oldDump(t), filepath.Join(t.TempDir(), "x.nup"), PackOptions{Level: 3, Workers: 2}, func(done int64, total int64) {
		if done > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err %v", err)
	}
}

// Unpacking checks every file, and refuses a name that is not one.
func TestUnpackRefusesDamageAndBadNames(t *testing.T) {
	packages := game()
	nup := testnup.Build(packages)
	offset, _ := testnup.Extent(nup, packages, 0, "romfs/Data/noise.bin")
	nup[offset+100] ^= 1
	if _, err := Unpack(context.Background(), write(t, nup), filepath.Join(t.TempDir(), "out"), 2, 2, nil); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("damaged: %v", err)
	}
	for _, name := range []string{"romfs/../escape.bin", "romfs/a\\b.bin", "romfs/./x"} {
		nup := testnup.Build([]testnup.Package{{TitleId: 1, Files: []testnup.File{{Path: name, Data: []byte("x")}}}})
		dest := filepath.Join(t.TempDir(), "out")
		if _, err := Unpack(context.Background(), write(t, nup), dest, 1, 1, nil); err == nil || !strings.Contains(err.Error(), "cannot be written here") {
			t.Errorf("%q: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.bin")); err == nil {
			t.Errorf("%q: written outside", name)
		}
	}
	// two updates of the same version would write their files to the same place: refused before
	// anything is written
	same := testnup.Build([]testnup.Package{
		{TitleId: 1, Kind: testnup.Base, Files: []testnup.File{{Path: "exefs/main", Data: []byte("game")}}},
		{TitleId: 1, Kind: testnup.Update, Version: 65536, Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte("one")}}},
		{TitleId: 1, Kind: testnup.Update, Version: 65536, Files: []testnup.File{{Path: "romfs/a.bin", Data: []byte("two")}}},
	})
	clash := filepath.Join(t.TempDir(), "out")
	if _, err := Unpack(context.Background(), write(t, same), clash, 1, 1, nil); err == nil || !strings.Contains(err.Error(), "same place") {
		t.Errorf("two updates of the same version: %v", err)
	}
	if _, err := os.Stat(clash); err == nil {
		t.Error("nothing is written when two files clash")
	}
	// names that differ only in case are one where the system does not tell them apart
	cased := testnup.Build([]testnup.Package{{TitleId: 1, Kind: testnup.Base, Files: []testnup.File{
		{Path: "romfs/Data/A.bin", Data: []byte("upper")}, {Path: "romfs/data/a.bin", Data: []byte("lower")}}}})
	_, err := Unpack(context.Background(), write(t, cased), filepath.Join(t.TempDir(), "out"), 1, 1, nil)
	foldsCase := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
	if foldsCase && (err == nil || !strings.Contains(err.Error(), "same place")) || !foldsCase && err != nil {
		t.Errorf("names that differ only in case on %s: %v", runtime.GOOS, err)
	}

	// a folder already there is not written into
	dest := t.TempDir()
	if _, err := Unpack(context.Background(), write(t, testnup.Build(game())), dest, 1, 1, nil); !errors.Is(err, fs.ErrExist) {
		t.Errorf("into a folder there: %v", err)
	}
}

// Big files, on one reader or several: the same big data in the game and its update is kept
// once, big files of the same size and other data are both kept, big files zstd makes smaller
// are streamed and the others written by copiers; all of them read back whole.
func TestPackBigFiles(t *testing.T) {
	same := noise(9<<20+3, 21)
	other := noise(9<<20+3, 22)
	text := bytes.Repeat([]byte("big text that zstd makes smaller "), 400_000)
	root := filepath.Join(t.TempDir(), "dump")
	writeTree(t, root, map[string][]byte{
		"base/exefs/main.npdm":          npdm(0x0100000000010000),
		"base/romfs/a/same.bin":         same,
		"base/romfs/a/other.bin":        other,
		"base/romfs/a/text.bin":         text,
		"base/romfs/a/unique.bin":       noise(10<<20+7, 23),
		"updates/65536/romfs/same.bin":  same,
		"updates/65536/romfs/text.bin":  text,
		"updates/65536/romfs/small.bin": []byte("small"),
	})
	for _, o := range []PackOptions{{Level: 3, Workers: 4, Readers: 1}, {Level: 3, Workers: 6, Readers: 6}, {Level: 0, Workers: 3, Readers: 2}} {
		out := filepath.Join(t.TempDir(), "big.nup")
		result, err := Pack(context.Background(), root, out, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.Shared != 2 || result.Files != 8 {
			t.Errorf("%+v: %d shared of %d files", o, result.Shared, result.Files)
		}
		// same.bin and text.bin are kept once: the NUP holds a little more than the other data
		if limit := int64(len(same)+len(other)+len(text)+(10<<20+7)) + 1<<20; result.FileSize > limit {
			t.Errorf("%+v: %d bytes, more than %d", o, result.FileSize, limit)
		}
		if _, check, err := Verify(context.Background(), out, 3, 2, nil); err != nil || len(check.Damaged) != 0 {
			t.Fatalf("%+v: %v %+v", o, err, check.Damaged)
		}
		dest := filepath.Join(t.TempDir(), "back")
		if _, err := Unpack(context.Background(), out, dest, 4, 3, nil); err != nil {
			t.Fatal(err)
		}
		compareTrees(t, root, dest)
	}
}

// fullDisk is a file that takes some bytes, then fails as a full disk does.
type fullDisk struct {
	file *os.File
	left *atomic.Int64
}

func (f *fullDisk) Write(p []byte) (int, error) {
	if f.left.Add(-int64(len(p))) < 0 {
		return 0, errors.New("there is not enough space on the disk")
	}
	return f.file.Write(p)
}

func (f *fullDisk) Close() error {
	return f.file.Close()
}

// A disk that fills up while a NUP is unpacked ends the work with its error, on one reader or
// several, with zstd or raw data, and leaves nothing behind.
func TestUnpackOnAFullDisk(t *testing.T) {
	before := createOutput
	t.Cleanup(func() { createOutput = before })
	src := oldDump(t)
	for _, level := range []int{3, 0} {
		out, _ := pack(t, src, PackOptions{Level: level, Workers: 3, Readers: 3})
		for _, room := range []int64{0, 100, 3 << 20, 12 << 20} {
			for _, readers := range []int{1, 4} {
				left := &atomic.Int64{}
				left.Store(room)
				createOutput = func(path string) (io.WriteCloser, error) {
					file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
					return &fullDisk{file: file, left: left}, err
				}
				dest := filepath.Join(t.TempDir(), "out")
				done := make(chan error, 1)
				go func() {
					_, err := Unpack(context.Background(), out, dest, 4, readers, nil)
					done <- err
				}()
				select {
				case err := <-done:
					if err == nil || !strings.Contains(err.Error(), "not enough space") {
						t.Errorf("level %d, room %d, %d readers: %v", level, room, readers, err)
					}
				case <-time.After(30 * time.Second):
					t.Fatalf("level %d, room %d, %d readers: the unpacking hangs", level, room, readers)
				}
				if _, err := os.Stat(dest); err == nil {
					t.Errorf("level %d, room %d: the folder is left behind", level, room)
				}
			}
		}
	}
}
