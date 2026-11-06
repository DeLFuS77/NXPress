package nup

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// playable is a game and its update as a dump gives them, with main and main.npdm in each ExeFS.
func playable() []testnup.Package {
	packages := game()
	packages[1].Files = append(packages[1].Files, testnup.File{Path: "exefs/main.npdm", Data: []byte("NPDM of the update")})
	return packages
}

func emulatorsOf(t *testing.T, nup []byte) Emulators {
	t.Helper()
	container, err := Read(bytes.NewReader(nup), int64(len(nup)))
	if err != nil {
		t.Fatal(err)
	}
	return container.Emulators()
}

func TestEmulatorsOpenAGoodNup(t *testing.T) {
	if got := emulatorsOf(t, testnup.Build(playable())); len(got.Refused) != 0 || got.NoStart != "" {
		t.Errorf("%+v", got)
	}
	// what NXPress packs meets them
	out, _ := pack(t, oldDump(t), PackOptions{Level: 3, Workers: 2})
	container, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := container.Emulators(); len(got.Refused) != 0 || got.NoStart != "" {
		t.Errorf("packed: %+v", got)
	}
}

// What the format and NXPress let be, and the emulators refuse.
func TestEmulatorsRefuseWhatTheyDoNotKnow(t *testing.T) {
	le := binary.LittleEndian
	cases := map[string]struct {
		change func(nup []byte)
		want   string
	}{
		"kind 3": {func(nup []byte) { nup[headerSize+tocHeadSize+2*packageSize+0x18] = 3; testnup.Rehash(nup) }, "package 2 is of kind 3"},
		"flags":  {func(nup []byte) { nup[headerSize+tocHeadSize+0x19] = 0x02; testnup.Rehash(nup) }, "package 0 has the flags 0x2"},
		"codec flags": {func(nup []byte) { setFile(nup, 3, 0, func(record []byte) { record[0x41] = 1 }) },
			"1 file has codec flags"},
		"empty with data": {func(nup []byte) {
			for i := range 20 {
				setFile(nup, 3, i, func(record []byte) {
					if le.Uint64(record[0x08:]) == 0 {
						le.PutUint64(record[0x10:], 0x1000)
					}
				})
			}
		}, "1 file of 0 bytes with a place for data"},
	}
	for name, c := range cases {
		nup := testnup.Build(playable())
		c.change(nup)
		got := emulatorsOf(t, nup)
		if len(got.Refused) != 1 || !strings.Contains(got.Refused[0], c.want) {
			t.Errorf("%s: %+v", name, got)
		}
	}
	// the files marked with the high bit, as a build of the packer of the format writes them
	tagged := testnup.BuildWith(playable(), testnup.Options{TagFiles: true, Dedup: true})
	if got := emulatorsOf(t, tagged); len(got.Refused) != 1 || !strings.Contains(got.Refused[0], "high bit") {
		t.Errorf("tagged: %+v", got)
	}
	// names by language that run past their record, which NXPress tells as damaged metadata
	packages := playable()
	packages[2].Meta = []testnup.Record{{Type: testnup.LocalizedTitles, Payload: []byte{5, 'e', 'n'}}}
	if got := emulatorsOf(t, testnup.Build(packages)); len(got.Refused) != 1 || !strings.Contains(got.Refused[0], "by language of package 2") {
		t.Errorf("languages: %+v", got)
	}
}

// A folder of the table inside itself is refused, even one no package holds.
func TestEmulatorsRefuseALoop(t *testing.T) {
	nup := testnup.Build(playable())
	le := binary.LittleEndian
	toc := nup[headerSize:]
	dirs, files := le.Uint64(toc[0x08:]), le.Uint64(toc[0x10:])
	children := tocHeadSize + 3*packageSize + dirs*dirSize + files*fileSize
	le.PutUint32(toc[children:], 0)
	testnup.Rehash(nup)
	if got := emulatorsOf(t, nup); len(got.Refused) != 1 || !strings.Contains(got.Refused[0], "inside itself") {
		t.Errorf("%+v", got)
	}
}

func TestEmulatorsNeedMainAndItsNpdm(t *testing.T) {
	// the newest update has an ExeFS without main.npdm: it is the one they take
	if got := emulatorsOf(t, testnup.Build(game())); got.NoStart != "the ExeFS of the update v131072 lacks: main.npdm" {
		t.Errorf("update: %q", got.NoStart)
	}
	// an update without an ExeFS: the one of the game
	packages := playable()
	packages[1].Files = []testnup.File{{Path: "romfs/Data/patch.bin", Data: []byte("patched")}}
	packages[0].Files = packages[0].Files[1:]
	if got := emulatorsOf(t, testnup.Build(packages)); got.NoStart != "the ExeFS of the game v65536 lacks: main" {
		t.Errorf("game: %q", got.NoStart)
	}
	// DLC only, and a game without an ExeFS
	dlc := testnup.Build(playable()[2:])
	if got := emulatorsOf(t, dlc); !strings.Contains(got.NoStart, "no game or update") {
		t.Errorf("DLC: %q", got.NoStart)
	}
	romfs := testnup.Build([]testnup.Package{{TitleId: 1, Files: []testnup.File{{Path: "romfs/a", Data: []byte("a")}}}})
	if got := emulatorsOf(t, romfs); got.NoStart != "no game or update in it has an ExeFS" {
		t.Errorf("romfs only: %q", got.NoStart)
	}
	// an update alone starts on its own
	alone := playable()[1:2]
	alone[0].Flags = 1
	if got := emulatorsOf(t, testnup.Build(alone)); got.NoStart != "" {
		t.Errorf("update alone: %q", got.NoStart)
	}
}

// A NUP NXPress does not read for a reason the emulators share says so.
func TestEmulatorsRefuseWhatNXPressRefuses(t *testing.T) {
	for name, change := range map[string]func(nup []byte){
		"flags":       func(nup []byte) { nup[0x4C] = 1 },
		"unaligned":   func(nup []byte) { setFile(nup, 3, 0, func(record []byte) { record[0x10]++ }) },
		"codec":       func(nup []byte) { setFile(nup, 3, 0, func(record []byte) { record[0x40] = 2 }) },
		"kind":        func(nup []byte) { nup[headerSize+tocHeadSize+0x18] = 9; testnup.Rehash(nup) },
		"bad records": nil,
	} {
		var nup []byte
		if change == nil {
			packages := playable()
			packages[0].Meta = append(packages[0].Meta, testnup.Record{Type: 0x20, Payload: []byte("x")})
			nup = breakLastRecord(t, testnup.Build(packages))
		} else {
			nup = testnup.Build(playable())
			change(nup)
		}
		_, err := Read(bytes.NewReader(nup), int64(len(nup)))
		if err == nil || !EmulatorsRefuse(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a NUP cut short is not one they are said to refuse: it is a copy that stopped
	nup := testnup.Build(playable())
	if _, err := Read(bytes.NewReader(nup[:len(nup)-1]), int64(len(nup)-1)); err == nil || EmulatorsRefuse(err) {
		t.Errorf("cut: %v", err)
	}
}

// breakLastRecord makes the size of the last record of the metadata of the game run past its
// block.
func breakLastRecord(t *testing.T, nup []byte) []byte {
	t.Helper()
	le := binary.LittleEndian
	record := nup[headerSize+tocHeadSize:]
	offset, size := le.Uint64(record[0x20:]), uint64(le.Uint32(record[0x28:]))
	block := nup[offset : offset+size]
	for at := 0; at+tlvHeadSize <= len(block); {
		length := int(le.Uint32(block[at+4:]))
		if at+tlvHeadSize+length == len(block) {
			le.PutUint32(block[at+4:], uint32(length+64))
			return nup
		}
		at += tlvHeadSize + length
	}
	t.Fatal("no last record")
	return nil
}
