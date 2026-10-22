package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDat(t *testing.T) {
	folder := t.TempDir()
	dump := []byte(strings.Repeat("game card", 1000))
	other := []byte(strings.Repeat("other dump", 800))
	sha := sha1.Sum(dump)
	path := filepath.Join(folder, "game.xci")
	os.WriteFile(path, dump, 0o644)
	odd := filepath.Join(folder, "odd.xci")
	os.WriteFile(odd, append([]byte(nil), dump[:len(dump)-1]...), 0o644)
	same := filepath.Join(folder, "same.xci")
	changed := append([]byte(nil), dump...)
	changed[0] ^= 1
	os.WriteFile(same, changed, 0o644)

	datPath := filepath.Join(folder, "switch.dat")
	os.WriteFile(datPath, []byte(fmt.Sprintf(`<?xml version="1.0"?>
<datafile>
	<header><name>Nintendo - Nintendo Switch</name></header>
	<game name="Some Game (Europe)"><rom name="Some Game (Europe).xci" size="%d" crc="%08x" sha1="%s"/></game>
	<game name="Other Game (USA)"><rom name="Other Game (USA).xci" size="%d" crc="%08x"/></game>
</datafile>`, len(dump), crc32.ChecksumIEEE(dump), hex.EncodeToString(sha[:]), len(other), crc32.ChecksumIEEE(other))), 0o644)

	d, err := loadDat(datPath)
	if err != nil || d.count != 2 || d.name != "Nintendo - Nintendo Switch" {
		t.Fatalf("DAT: %+v %v", d, err)
	}
	for file, want := range map[string]string{
		path: "in the DAT: Some Game (Europe)",
		odd:  "not in the DAT: no dump of its size",
		same: "not in the DAT: 1 dumps of its size, with other hashes",
	} {
		if got, err := d.match(context.Background(), file, nil); err != nil || got != want {
			t.Fatalf("%s: %q %v, want %q", filepath.Base(file), got, err, want)
		}
	}

	os.WriteFile(datPath, []byte(`<datafile><game name="x"><rom name="x" size="10"/></game></datafile>`), 0o644)
	if _, err := loadDat(datPath); err == nil {
		t.Fatal("a DAT without hashes")
	}
}
