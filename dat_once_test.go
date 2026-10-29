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

// With a DAT, a file it lists by its SHA-1 is that dump: Verify reads it once, to compare it,
// and does not read it again to check its NCA files (this one has none: it is no NSP). Listed
// by its CRC only, it is checked as well.
func TestVerifyWithADatReadsADumpOnce(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	dump := []byte(strings.Repeat("game card", 1000))
	sum := sha1.Sum(dump)
	path := filepath.Join(folder, "game.xci")
	os.WriteFile(path, dump, 0o644)
	write := func(hashes string) *dat {
		datPath := filepath.Join(folder, "switch.dat")
		os.WriteFile(datPath, []byte(fmt.Sprintf(`<datafile><game name="Some Game"><rom name="Some Game.xci" size="%d" %s/></game></datafile>`, len(dump), hashes)), 0o644)
		d, err := loadDat(datPath)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	o := options{datFile: write(fmt.Sprintf(`sha1="%s"`, hex.EncodeToString(sum[:])))}
	if _, note, err := doJob(context.Background(), "verify", path, o, 1, quiet{}); err != nil || note != "in the DAT: Some Game" {
		t.Fatalf("listed by its SHA-1: %q %v", note, err)
	}
	o = options{datFile: write(fmt.Sprintf(`crc="%08x"`, crc32.ChecksumIEEE(dump)))}
	if _, _, err := doJob(context.Background(), "verify", path, o, 1, quiet{}); err == nil {
		t.Fatal("listed by its CRC only, the file is not checked")
	}
}
