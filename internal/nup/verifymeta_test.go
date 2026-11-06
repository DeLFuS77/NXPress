package nup

import (
	"context"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

func TestVerifyLooksAtTheMetadata(t *testing.T) {
	npdmFile := append([]byte("META"), make([]byte, 0x80)...)
	otherNpdm := append([]byte("META"), make([]byte, 0x81)...)
	main := make([]byte, 0x100)
	copy(main, "NSO0")
	for i := 0x40; i < 0x60; i++ {
		main[i] = byte(i)
	}
	packages := []testnup.Package{
		{TitleId: 0x0400486024DEC000, Kind: testnup.Base, Version: 0,
			Meta: []testnup.Record{
				{Type: 0x0001, Payload: otherNpdm},
				{Type: 0x0007, Payload: make([]byte, 31)},
				{Type: 0x0005, Payload: []byte{0xFF, 0xFE, 'x'}},
				{Type: 0x0003, Payload: []byte("GIF89a")},
				{Type: 0x0009, Payload: []byte{2, 'j', 'a', 9, 0, 0, 0, 'x'}},
			},
			Files: []testnup.File{
				{Path: "exefs/main", Data: main, Zstd: true},
				{Path: "exefs/main.npdm", Data: npdmFile},
			}},
		{TitleId: 0x0400486024DEC000, Kind: testnup.Update, Version: 65536,
			Meta: []testnup.Record{
				{Type: 0x0001, Payload: npdmFile},
				{Type: 0x0007, Payload: append(make([]byte, 0, 32), main[0x40:0x60]...)},
				{Type: 0x0005, Payload: []byte("Made-up Quest")},
			},
			Files: []testnup.File{
				{Path: "exefs/main", Data: main, Zstd: true},
				{Path: "exefs/main.npdm", Data: npdmFile},
			}},
		// a DLC of a packer that counted the types from 0: its source dump (7) reads as a build
		// ID, its build ID (6) as a publisher, which are not looked at without an NPDM
		{TitleId: 0x0400486024DED001, Kind: testnup.Addon, Version: 0,
			Meta: []testnup.Record{
				{Type: 0x0007, Payload: []byte("dlc folder")},
				{Type: 0x0006, Payload: []byte{0xFF, 0xFE, 0xFD}},
			},
			Files: []testnup.File{{Path: "romfs/dlc.bin", Data: []byte("dlc")}}},
	}
	path := write(t, testnup.Build(packages))
	_, check, err := Verify(context.Background(), path, 2, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	damaged := map[string]string{}
	for _, d := range check.Damaged {
		damaged[d.Package+" | "+d.Path] = d.Reason
	}
	game := Label(Package{TitleID: 0x0400486024DEC000, Kind: KindBase})
	for _, want := range []string{
		game + " | metadata: build ID",
		game + " | metadata: name",
		game + " | metadata: records of languages",
	} {
		if _, ok := damaged[want]; !ok {
			t.Errorf("%q not found in %v", want, damaged)
		}
	}
	if len(damaged) != 3 {
		t.Errorf("damaged %v", damaged)
	}
	notes := strings.Join(check.Notes, "\n")
	for _, want := range []string{"NPDM of its metadata differs", "icon is neither JPEG nor PNG"} {
		if !strings.Contains(notes, want) {
			t.Errorf("no note %q in %q", want, notes)
		}
	}
	if len(check.Notes) != 2 {
		t.Errorf("notes %q", check.Notes)
	}
}

func TestVerifyTellsABuildIDOtherThanTheOneOfMain(t *testing.T) {
	npdmFile := append([]byte("META"), make([]byte, 0x80)...)
	main := make([]byte, 0x100)
	copy(main, "NSO0")
	packages := []testnup.Package{
		{TitleId: 0x0400486024DEC000, Kind: testnup.Base,
			Meta: []testnup.Record{{Type: 0x0001, Payload: npdmFile}, {Type: 0x0007, Payload: make([]byte, 32)}},
			Files: []testnup.File{
				{Path: "exefs/main", Data: append(main[:0x40:0x40], append([]byte{1}, main[0x41:]...)...)},
				{Path: "exefs/main.npdm", Data: npdmFile},
			}},
	}
	_, check, err := Verify(context.Background(), write(t, testnup.Build(packages)), 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(check.Damaged) != 0 || len(check.Notes) != 1 || !strings.Contains(check.Notes[0], "build ID of its metadata differs") {
		t.Errorf("%+v %q", check.Damaged, check.Notes)
	}
}
