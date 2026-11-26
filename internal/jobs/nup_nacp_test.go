package jobs

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// The card of a NUP in Details tells a package whose NACP keeps its names compressed and they
// cannot be read, and gives the name of an update whose NACP keeps them compressed and readable.
func TestNupCardCompressedNacp(t *testing.T) {
	compressed := func(name string) []byte {
		entries := make([]byte, 32*0x300)
		copy(entries, name)
		copy(entries[0x200:], "Nobody Games")
		var packed bytes.Buffer
		writer, _ := flate.NewWriter(&packed, flate.BestCompression)
		writer.Write(entries)
		writer.Close()
		nacp := make([]byte, 0x4000)
		binary.LittleEndian.PutUint16(nacp, uint16(packed.Len()))
		copy(nacp[2:], packed.Bytes())
		nacp[0x3215] = 1
		return nacp
	}
	broken := compressed("Made-up Quest")
	clear(broken[:0x200])
	copy(broken, "Renamed Quest")
	files := []testnup.File{{Path: "romfs/a.bin", Data: []byte("a")}}
	packages := []testnup.Package{
		{TitleId: 0x0100ABCD00010000, Kind: testnup.Base, Meta: []testnup.Record{{Type: testnup.Nacp, Payload: compressed("Made-up Quest")}}, Files: files},
		{TitleId: 0x0100ABCD00010000, Kind: testnup.Update, Version: 65536, Meta: []testnup.Record{{Type: testnup.Nacp, Payload: compressed("Made-up Quest")}}, Files: files},
		{TitleId: 0x0100ABCD00010000, Kind: testnup.Update, Version: 131072, Meta: []testnup.Record{{Type: testnup.Nacp, Payload: broken}}, Files: files},
	}
	path := filepath.Join(t.TempDir(), "quest.nup")
	if err := os.WriteFile(path, testnup.Build(packages), 0o644); err != nil {
		t.Fatal(err)
	}
	details, err := NupCard(path)
	if err != nil {
		t.Fatal(err)
	}
	cards := details.Packages
	if len(cards) != 3 {
		t.Fatalf("%+v", cards)
	}
	for i, want := range []struct {
		name, publisher string
		unreadable      bool
	}{{"Made-up Quest", "Nobody Games", false}, {"Made-up Quest", "Nobody Games", false}, {"", "", true}} {
		if cards[i].Name != want.name || cards[i].Publisher != want.publisher || cards[i].NamesUnreadable != want.unreadable {
			t.Errorf("package %d: %+v", i, cards[i])
		}
	}
}
