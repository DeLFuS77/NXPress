package cli

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/DeLFuS77/NXPress/internal/dat"
)

// A DAT is read in the encodings DAT tools write: UTF-8 with or without its mark, UTF-16 by
// its mark, ISO-8859-1 and windows-1252 by its declaration. One of clrmamepro, which is not XML,
// is told for what it is.
func TestDatEncodings(t *testing.T) {
	folder := t.TempDir()
	dump := []byte(strings.Repeat("game card", 100))
	game := filepath.Join(folder, "game.xci")
	os.WriteFile(game, dump, 0o644)
	datText := func(encoding string, name string) string {
		return fmt.Sprintf(`<?xml version="1.0" encoding="%s"?>
<datafile><header><name>Nintendo - Nintendo Switch</name></header>
<game name="%s"><rom name="x.xci" size="%d" crc="%08x"/></game></datafile>`, encoding, name, len(dump), crc32.ChecksumIEEE(dump))
	}
	utf16Bytes := func(text string, order binary.AppendByteOrder, mark bool) []byte {
		data := []byte{}
		units := utf16.Encode([]rune(text))
		if mark {
			units = append([]uint16{0xFEFF}, units...)
		}
		for _, unit := range units {
			data = order.AppendUint16(data, unit)
		}
		return data
	}
	for label, c := range map[string]struct {
		data []byte
		want string
	}{
		"UTF-8":             {[]byte(datText("UTF-8", "Pokémon")), "Pokémon"},
		"UTF-8 with mark":   {append([]byte{0xEF, 0xBB, 0xBF}, datText("UTF-8", "Pokémon")...), "Pokémon"},
		"ISO-8859-1":        {[]byte(datText("ISO-8859-1", "Pok\xe9mon")), "Pokémon"},
		"windows-1252":      {[]byte(datText("windows-1252", "Assassin\x92s Creed \x80")), "Assassin’s Creed €"},
		"UTF-16 LE":         {utf16Bytes(datText("UTF-16", "ポケモン"), binary.LittleEndian, true), "ポケモン"},
		"UTF-16 BE":         {utf16Bytes(datText("UTF-16", "ポケモン"), binary.BigEndian, true), "ポケモン"},
		"UTF-16 LE no mark": {utf16Bytes(datText("UTF-16", "Pokémon"), binary.LittleEndian, false), "Pokémon"},
	} {
		path := filepath.Join(folder, "test.dat")
		os.WriteFile(path, c.data, 0o644)
		d, err := dat.LoadDat(path)
		if err != nil {
			t.Errorf("%s: %v", label, err)
			continue
		}
		if found, err := d.Match(context.Background(), game, nil); err != nil || found != "in the DAT: "+c.want || d.Name != "Nintendo - Nintendo Switch" {
			t.Errorf("%s: %q %q %v", label, found, d.Name, err)
		}
	}

	path := filepath.Join(folder, "clrmamepro.dat")
	os.WriteFile(path, []byte("clrmamepro (\n\tname \"Nintendo - Nintendo Switch\"\n)\n\ngame (\n\tname \"x\"\n)\n"), 0o644)
	if _, err := dat.LoadDat(path); err != dat.ErrClrmamepro {
		t.Fatalf("clrmamepro: %v", err)
	}
	os.WriteFile(path, []byte(datText("Shift_JIS", "x")), 0o644)
	if _, err := dat.LoadDat(path); err == nil || !strings.Contains(err.Error(), "DATs in UTF-8, UTF-16, ISO-8859-1 and windows-1252 can be read") {
		t.Fatalf("another encoding: %v", err)
	}
}
