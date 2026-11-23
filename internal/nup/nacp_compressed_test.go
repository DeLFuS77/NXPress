package nup

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// a NACP that keeps its names compressed gives each language its own name, Polish and Thai too,
// and the publisher of American English
func TestControlRecordsCompressedNames(t *testing.T) {
	entries := make([]byte, 32*nacpEntry)
	copy(entries[0:], "Game")
	copy(entries[0x200:], "Publisher")
	copy(entries[2*nacpEntry:], "Gemu")
	copy(entries[16*nacpEntry:], "Gra")
	copy(entries[17*nacpEntry:], "Kem")
	var packed bytes.Buffer
	writer, err := flate.NewWriter(&packed, flate.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	writer.Write(entries)
	writer.Close()
	nacp := make([]byte, nacpSize)
	binary.LittleEndian.PutUint16(nacp, uint16(packed.Len()))
	copy(nacp[2:], packed.Bytes())
	nacp[0x3215] = 1
	copy(nacp[nacpVersionAt:], "1.0.0")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "control.nacp"), nacp, 0o644); err != nil {
		t.Fatal(err)
	}

	records := controlRecords(dir)
	if got := string(records[tlvTitleName]); got != "Game" {
		t.Fatalf("name = %q, want Game", got)
	}
	if got := string(records[tlvPublisher]); got != "Publisher" {
		t.Fatalf("publisher = %q, want Publisher", got)
	}
	titles, err := parseLocalized(records[tlvLocalizedTitles])
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"en-us": "Game", "ja": "Gemu", "pl": "Gra", "th": "Kem"}
	if len(titles) != len(want) {
		t.Fatalf("titles = %d languages, want %d", len(titles), len(want))
	}
	for tag, name := range want {
		if got := string(titles[tag]); got != name {
			t.Fatalf("title %s = %q, want %q", tag, got, name)
		}
	}
}
