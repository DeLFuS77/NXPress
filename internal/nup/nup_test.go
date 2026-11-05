package nup

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// noise is data that zstd cannot make smaller, of a size.
func noise(size int, seed int64) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(seed)).Read(data)
	return data
}

// game is a game with an update and a DLC, files raw and with zstd, one of them bigger than what
// is read at once, and a file the update shares with the game.
func game() []testnup.Package {
	shared := bytes.Repeat([]byte("shared data of the game "), 4000)
	return []testnup.Package{
		{TitleId: 0x0400486024DEC000, Kind: testnup.Base, Version: 65536,
			Meta: []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Quest [0400486024DEC000][v65536]")},
				{Type: testnup.Publisher, Payload: []byte("Nobody Games")}, {Type: testnup.DisplayVersion, Payload: []byte("1.0.0")},
				{Type: testnup.Icon, Payload: testnup.Jpeg}},
			Files: []testnup.File{
				{Path: "exefs/main", Data: noise(3000, 1)},
				{Path: "exefs/main.npdm", Data: []byte("NPDM made up")},
				{Path: "romfs/Data/big.bin", Data: bytes.Repeat([]byte("0123456789abcdef"), 300_000), Zstd: true},
				{Path: "romfs/Data/noise.bin", Data: noise(5<<20/2, 2)},
				{Path: "romfs/Data/shared.bin", Data: shared, Zstd: true},
				{Path: "romfs/empty.txt", Data: nil},
				{Path: "romfs/Sound/a/b/c/deep.bin", Data: []byte("deep"), Zstd: true},
			}},
		{TitleId: 0x0400486024DEC000, Kind: testnup.Update, Version: 131072,
			Meta: []testnup.Record{{Type: testnup.DisplayVersion, Payload: []byte("1.0.2")}},
			Files: []testnup.File{
				{Path: "exefs/main", Data: noise(3200, 3)},
				{Path: "romfs/Data/shared.bin", Data: shared, Zstd: true},
				{Path: "romfs/Data/patch.bin", Data: []byte("patched"), Zstd: true},
			}},
		{TitleId: 0x0400486024DED001, Kind: testnup.Addon, Version: 0,
			Meta:  []testnup.Record{{Type: testnup.LocalizedTitles, Payload: testnup.Localized("ja", "追加", "en-us", "Extra Pack")}},
			Files: []testnup.File{{Path: "romfs/dlc.bin", Data: noise(100, 4)}}},
	}
}

func write(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.nup")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenTellsWhatANupHolds(t *testing.T) {
	packages := game()
	for _, o := range []testnup.Options{{Dedup: true}, {TagFiles: true}} {
		path := write(t, testnup.BuildWith(packages, o))
		container, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(container.Packages) != 3 {
			t.Fatalf("%d packages", len(container.Packages))
		}
		base, update, dlc := container.Packages[0], container.Packages[1], container.Packages[2]
		if base.TitleIDText() != "0400486024DEC000" || update.TitleIDText() != "0400486024DEC800" || dlc.TitleIDText() != "0400486024DED001" {
			t.Errorf("IDs %s %s %s", base.TitleIDText(), update.TitleIDText(), dlc.TitleIDText())
		}
		if base.Name() != "Made-up Quest" || base.Publisher() != "Nobody Games" || base.DisplayVersion() != "1.0.0" {
			t.Errorf("game %q %q %q", base.Name(), base.Publisher(), base.DisplayVersion())
		}
		if update.Name() != "" || update.DisplayVersion() != "1.0.2" || dlc.Name() != "Extra Pack" {
			t.Errorf("update %q %q, DLC %q", update.Name(), update.DisplayVersion(), dlc.Name())
		}
		if !bytes.Equal(container.Icon(), testnup.Jpeg) {
			t.Error("no icon")
		}
		if base.ExefsDir == NoDir || base.RomfsDir == NoDir || dlc.ExefsDir != NoDir {
			t.Errorf("folders %d %d %d", base.ExefsDir, base.RomfsDir, dlc.ExefsDir)
		}
		paths := []string{}
		if err := container.Walk(0, func(path string, _ int) error { paths = append(paths, path); return nil }); err != nil {
			t.Fatal(err)
		}
		want := "exefs/main exefs/main.npdm romfs/Data/big.bin romfs/Data/noise.bin romfs/Data/shared.bin romfs/Sound/a/b/c/deep.bin romfs/empty.txt"
		if strings.Join(paths, " ") != want {
			t.Errorf("paths %v", paths)
		}
		summary, err := container.Summarize(0)
		if err != nil {
			t.Fatal(err)
		}
		size := uint64(0)
		for _, file := range packages[0].Files {
			size += uint64(len(file.Data))
		}
		if summary.Files != 7 || summary.Size != size || summary.Stored >= summary.Size {
			t.Errorf("game summary %+v, size %d", summary, size)
		}
		totals := container.Totals()
		if totals.Files != 11 {
			t.Errorf("totals %+v", totals)
		}
		// the update shares its data with the game: kept once
		if o.Dedup {
			update, _ := container.Summarize(1)
			game, _ := container.Summarize(0)
			dlc, _ := container.Summarize(2)
			if totals.Stored >= game.Stored+update.Stored+dlc.Stored {
				t.Errorf("shared data counted twice: %d, %d+%d+%d", totals.Stored, game.Stored, update.Stored, dlc.Stored)
			}
		}
	}
}

func TestVerifyGoodNup(t *testing.T) {
	for _, o := range []testnup.Options{{Dedup: true}, {}, {TagFiles: true, Dedup: true}} {
		for _, workers := range []int{1, 4} {
			path := write(t, testnup.BuildWith(game(), o))
			calls, last := 0, int64(0)
			_, check, err := Verify(context.Background(), path, workers, workers, func(done int64, total int64) {
				calls++
				if done > total || done < 0 {
					t.Errorf("progress %d of %d", done, total)
				}
				last = done
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(check.Damaged) > 0 || check.Files != 11 || check.Unplaced != 0 || calls < 2 || last != check.Read || check.Read == 0 {
				t.Errorf("%+v: %+v, %d calls, last %d", o, check, calls, last)
			}
			if o.Dedup && check.Extents != 9 || !o.Dedup && check.Extents != 10 {
				t.Errorf("%+v: %d pieces of data read", o, check.Extents)
			}
		}
	}
}

// damage flips a byte of the data of a file.
func damage(nup []byte, packages []testnup.Package, pkg int, path string, at int) {
	offset, size := testnup.Extent(nup, packages, pkg, path)
	nup[offset+min(at, size-1)] ^= 0x55
}

func TestVerifyFindsDamagedFiles(t *testing.T) {
	packages := game()
	nup := testnup.Build(packages)
	damage(nup, packages, 0, "romfs/Data/noise.bin", 1<<20+7)
	damage(nup, packages, 0, "exefs/main", 0)
	// the data the update shares with the game: damaged in both
	damage(nup, packages, 0, "romfs/Data/shared.bin", 40)
	// zstd that cannot be decompressed any more
	damage(nup, packages, 0, "romfs/Data/big.bin", 2)
	_, check, err := Verify(context.Background(), write(t, nup), 2, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, d := range check.Damaged {
		found[d.Package+": "+d.Path] = d.Reason
	}
	want := []string{"game v65536: romfs/Data/noise.bin", "game v65536: exefs/main", "game v65536: romfs/Data/shared.bin",
		"update v131072: romfs/Data/shared.bin", "game v65536: romfs/Data/big.bin"}
	if len(found) != len(want) {
		t.Errorf("damaged %v", found)
	}
	for _, path := range want {
		if found[path] == "" {
			t.Errorf("%s not found damaged: %v", path, found)
		}
	}
	if found["game v65536: exefs/main"] != "its SHA-256 does not match" {
		t.Errorf("raw reason %q", found["game v65536: exefs/main"])
	}
	if reason := found["game v65536: romfs/Data/big.bin"]; !strings.Contains(reason, "zstd") && !strings.Contains(reason, "SHA-256") {
		t.Errorf("zstd reason %q", reason)
	}
}

func TestVerifyFindsZstdOfTheWrongSize(t *testing.T) {
	packages := []testnup.Package{{TitleId: 0x0100000000010000, Files: []testnup.File{
		{Path: "romfs/a.bin", Data: bytes.Repeat([]byte("a"), 5000), Zstd: true}}}}
	for _, size := range []uint64{4000, 6000} {
		nup := testnup.Build(packages)
		setFile(nup, 1, 0, func(record []byte) { binary.LittleEndian.PutUint64(record[0x08:], size) })
		_, check, err := Verify(context.Background(), write(t, nup), 1, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(check.Damaged) != 1 || !strings.Contains(check.Damaged[0].Reason, "decompresses to") {
			t.Errorf("size %d: %+v", size, check.Damaged)
		}
	}
}

// setFile changes the record of a file in the table of contents, and writes its hash again.
func setFile(nup []byte, packages int, index int, change func(record []byte)) {
	le := binary.LittleEndian
	toc := nup[headerSize:]
	dirs := int(le.Uint64(toc[0x08:]))
	change(toc[tocHeadSize+packages*packageSize+dirs*dirSize+index*fileSize:][:fileSize])
	testnup.Rehash(nup)
}

func TestVerifyRefusesADamagedTable(t *testing.T) {
	nup := testnup.Build(game())
	nup[headerSize+0x200] ^= 1
	_, _, err := Verify(context.Background(), write(t, nup), 1, 1, nil)
	if err == nil || !strings.Contains(err.Error(), "table of contents is damaged") {
		t.Errorf("err %v", err)
	}
}

func TestVerifyTellsAFileCutShort(t *testing.T) {
	nup := testnup.Build(game())
	for _, cut := range []int{len(nup) - 1, len(nup) / 2, 0x90, 10} {
		_, _, err := Verify(context.Background(), write(t, nup[:cut]), 1, 1, nil)
		if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("cut at %d: %v", cut, err)
		}
	}
}

func TestVerifyStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := Verify(ctx, write(t, testnup.Build(game())), 2, 2, func(done int64, total int64) {
		if done > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err %v", err)
	}
}

func TestReadRefusesMalformedTables(t *testing.T) {
	cases := map[string]func(nup []byte){
		"codec 7": func(nup []byte) { setFile(nup, 3, 0, func(record []byte) { record[0x40] = 7 }) },
		"data outside the file": func(nup []byte) {
			setFile(nup, 3, 0, func(record []byte) { binary.LittleEndian.PutUint64(record[0x10:], uint64(len(nup))) })
		},
		"data far outside the file": func(nup []byte) {
			setFile(nup, 3, 0, func(record []byte) { binary.LittleEndian.PutUint64(record[0x10:], 1<<63) })
		},
		"data not aligned": func(nup []byte) {
			setFile(nup, 3, 0, func(record []byte) {
				binary.LittleEndian.PutUint64(record[0x10:], binary.LittleEndian.Uint64(record[0x10:])+1)
			})
		},
		"raw of another size": func(nup []byte) {
			setFile(nup, 3, 0, func(record []byte) { binary.LittleEndian.PutUint64(record[0x18:], 1) })
		},
		"a name outside the pool": func(nup []byte) {
			setFile(nup, 3, 0, func(record []byte) { binary.LittleEndian.PutUint32(record[0x00:], 0xFFFFFF) })
		},
		"a child that is not there": func(nup []byte) {
			le := binary.LittleEndian
			toc := nup[headerSize:]
			pool := le.Uint64(toc[0x18:])
			le.PutUint32(toc[pool-4:], 0x7FFFFFFF)
			testnup.Rehash(nup)
		},
		"an unknown kind": func(nup []byte) { nup[headerSize+tocHeadSize+0x18] = 9; testnup.Rehash(nup) },
		"a root outside": func(nup []byte) {
			binary.LittleEndian.PutUint32(nup[headerSize+tocHeadSize+0x08:], 0xFFFF)
			testnup.Rehash(nup)
		},
		"flags":   func(nup []byte) { nup[0x4C] = 1 },
		"version": func(nup []byte) { nup[3] = 2 },
	}
	for name, change := range cases {
		nup := testnup.Build(game())
		change(nup)
		if _, err := Read(bytes.NewReader(nup), int64(len(nup))); err == nil {
			t.Errorf("%s: read", name)
		}
	}
}

func TestWalkRefusesALoop(t *testing.T) {
	nup := testnup.Build(game())
	// the first child of the root (exefs) made the root itself
	le := binary.LittleEndian
	toc := nup[headerSize:]
	dirs, files := le.Uint64(toc[0x08:]), le.Uint64(toc[0x10:])
	children := tocHeadSize + 3*packageSize + dirs*dirSize + files*fileSize
	le.PutUint32(toc[children:], 0)
	testnup.Rehash(nup)
	container, err := Read(bytes.NewReader(nup), int64(len(nup)))
	if err != nil {
		t.Fatal(err)
	}
	if err := container.Walk(0, func(string, int) error { return nil }); err == nil || !strings.Contains(err.Error(), "inside itself") {
		t.Errorf("err %v", err)
	}
	if _, _, err := Verify(context.Background(), write(t, nup), 1, 1, nil); err == nil {
		t.Error("verified")
	}
}

func TestVerifyEmptyPackageAndEmptyFile(t *testing.T) {
	nup := testnup.Build([]testnup.Package{{TitleId: 1, Kind: testnup.Update, Flags: 1, Files: []testnup.File{{Path: "romfs/zero"}}}})
	container, check, err := Verify(context.Background(), write(t, nup), 4, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(check.Damaged) != 0 || check.Files != 1 || check.Extents != 0 || container.Packages[0].Name() != "" {
		t.Errorf("%+v", check)
	}
	// an empty file whose hash is not the one of nothing
	setFile(nup, 1, 0, func(record []byte) { record[0x20] ^= 1 })
	_, check, err = Verify(context.Background(), write(t, nup), 1, 1, nil)
	if err != nil || len(check.Damaged) != 1 {
		t.Errorf("%v %+v", err, check)
	}
}

// The packer of the format counted the types of the metadata from 0 for a while: such a block is
// read with its types where the format puts them.
func TestTypesCountedFromZero(t *testing.T) {
	nup := testnup.Build([]testnup.Package{{TitleId: 0x040074A01BF12000, Meta: []testnup.Record{
		{Type: 0, Payload: []byte("META npdm")}, {Type: 3, Payload: []byte("1.0.0")},
		{Type: 4, Payload: []byte("Made-up Bananza")}, {Type: 5, Payload: []byte("Nobody")}, {Type: 6, Payload: bytes.Repeat([]byte{9}, 32)},
	}, Files: []testnup.File{{Path: "romfs/a", Data: []byte("a")}}}})
	container, err := Read(bytes.NewReader(nup), int64(len(nup)))
	if err != nil {
		t.Fatal(err)
	}
	pkg := container.Packages[0]
	if pkg.Name() != "Made-up Bananza" || pkg.Publisher() != "Nobody" || pkg.DisplayVersion() != "1.0.0" || string(pkg.Meta[1]) != "META npdm" {
		t.Errorf("%q %q %q", pkg.Name(), pkg.Publisher(), pkg.DisplayVersion())
	}
}

func TestNames(t *testing.T) {
	nacp := make([]byte, 0x4000)
	copy(nacp[2*0x300:], "ゲーム")
	copy(nacp[2*0x300+0x200:], "会社")
	copy(nacp[0x3060:], "2.1.0")
	pkg := Package{Meta: map[uint16][]byte{tlvNacp: nacp}}
	if pkg.Name() != "ゲーム" || pkg.Publisher() != "会社" || pkg.DisplayVersion() != "2.1.0" {
		t.Errorf("%q %q %q", pkg.Name(), pkg.Publisher(), pkg.DisplayVersion())
	}
	pkg = Package{Meta: map[uint16][]byte{tlvTitleName: []byte("  Name\x01 [0100000000010000] [v0] "), tlvLocalizedTitles: testnup.Localized("fr", "Nom")}}
	if pkg.Name() != "Name" {
		t.Errorf("%q", pkg.Name())
	}
	pkg = Package{Meta: map[uint16][]byte{tlvLocalizedTitles: testnup.Localized("fr", "Nom")}}
	if pkg.Name() != "Nom" {
		t.Errorf("%q", pkg.Name())
	}
}

// Many small files, more than one read of the window holds, and files big enough to be read on
// their own, raw and with zstd: every one is checked, and a damaged one found wherever it is.
func TestVerifyManySmallAndBigFiles(t *testing.T) {
	files := []testnup.File{}
	for i := range 3000 {
		files = append(files, testnup.File{Path: "romfs/small/" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "/" + strconv.Itoa(i), Data: noise(4000+i%900, int64(i)), Zstd: i%3 == 0})
	}
	files = append(files, testnup.File{Path: "romfs/big/raw.bin", Data: noise(9<<20, 99)},
		testnup.File{Path: "romfs/big/zstd.bin", Data: append(noise(5<<20, 98), bytes.Repeat([]byte("z"), 4<<20)...), Zstd: true})
	packages := []testnup.Package{{TitleId: 0x0100000000010000, Files: files}}
	nup := testnup.Build(packages)
	for _, workers := range []int{1, 3, 8} {
		_, check, err := Verify(context.Background(), write(t, nup), workers, workers, nil)
		if err != nil || len(check.Damaged) != 0 || check.Files != 3002 {
			t.Fatalf("%d workers: %v %+v", workers, err, check.Damaged)
		}
	}
	for _, path := range []string{"romfs/big/raw.bin", "romfs/big/zstd.bin", files[1500].Path, files[2999].Path} {
		damaged := bytes.Clone(nup)
		offset, size := testnup.Extent(damaged, packages, 0, path)
		damaged[offset+size-1] ^= 0x80
		_, check, err := Verify(context.Background(), write(t, damaged), 4, 4, nil)
		if err != nil || len(check.Damaged) != 1 || check.Damaged[0].Path != path {
			t.Errorf("%s: %v %+v", path, err, check.Damaged)
		}
	}
}
