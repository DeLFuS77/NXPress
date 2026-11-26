package nup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// splitNup writes data in parts of size bytes (the last one shorter) in a folder named like a
// NUP, as FAT32 keeps a file over 4 GiB, and gives the folder.
func splitNup(t *testing.T, data []byte, size int) string {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "Made-up Game [0400AAAA00010000][v0].nup")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for part := 0; len(data) > 0; part++ {
		n := min(size, len(data))
		if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("%02d", part)), data[:n], 0o644); err != nil {
			t.Fatal(err)
		}
		data = data[n:]
	}
	return folder
}

func partsGame() []testnup.Package {
	return []testnup.Package{
		{TitleId: 0x0400AAAA00010000, Kind: testnup.Base, Meta: []testnup.Record{{Type: testnup.TitleName, Payload: []byte("Made-up Game")}},
			Files: []testnup.File{{Path: "romfs/a.bin", Data: bytes.Repeat([]byte("data of the game "), 2000), Zstd: true},
				{Path: "romfs/b.bin", Data: bytes.Repeat([]byte("more "), 3000)}, {Path: "exefs/main", Data: []byte("main")}}},
		{TitleId: 0x0400AAAA00010000, Kind: testnup.Update, Version: 65536,
			Files: []testnup.File{{Path: "romfs/a.bin", Data: bytes.Repeat([]byte("update "), 2000), Zstd: true}}},
	}
}

// A NUP in parts, given its folder, is read, verified and unpacked as the NUP whole.
func TestNupInPartsReadsAsOne(t *testing.T) {
	data := testnup.Build(partsGame())
	plain := filepath.Join(t.TempDir(), "Made-up Game.nup")
	if err := os.WriteFile(plain, data, 0o644); err != nil {
		t.Fatal(err)
	}
	whole, err := Open(plain)
	if err != nil {
		t.Fatal(err)
	}
	folder := splitNup(t, data, len(data)/3+1)
	if !IsNup(folder) {
		t.Fatal("the folder of parts is not told a NUP")
	}
	split, err := Open(folder)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(split.Packages, whole.Packages) || split.FileSize != whole.FileSize {
		t.Fatalf("read otherwise than the NUP whole: %+v, want %+v", split, whole)
	}
	for _, readers := range []int{1, 4} {
		_, check, err := Verify(context.Background(), folder, 4, readers, func(int64, int64) {})
		if err != nil || len(check.Damaged) > 0 || check.Files != 4 || check.Read == 0 {
			t.Fatalf("%d readers: %+v %v", readers, check, err)
		}
	}
	dest := filepath.Join(t.TempDir(), "unpacked")
	result, err := Unpack(context.Background(), folder, dest, 2, 1, func(int64, int64) {})
	if err != nil || result.Files != 4 {
		t.Fatalf("unpacked %+v: %v", result, err)
	}
}

// A damaged byte in a later part is found, and a part missing in the middle makes the NUP
// shorter than its table: it cannot be read.
func TestNupInPartsDamagedOrShort(t *testing.T) {
	data := testnup.Build(partsGame())
	size := len(data)/4 + 1
	damaged := bytes.Clone(data)
	damaged[len(damaged)-size/2] ^= 0x55
	folder := splitNup(t, damaged, size)
	if _, check, err := Verify(context.Background(), folder, 2, 1, func(int64, int64) {}); err == nil && len(check.Damaged) == 0 {
		t.Fatal("the damage in the last part is not found")
	}
	folder = splitNup(t, data, size)
	if err := os.Remove(filepath.Join(folder, "02")); err != nil {
		t.Fatal(err)
	}
	if _, check, err := Verify(context.Background(), folder, 2, 1, func(int64, int64) {}); err == nil && len(check.Damaged) == 0 {
		t.Fatal("a NUP with a part missing is sound")
	}
	if err := os.Remove(filepath.Join(folder, "00")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(folder); err == nil || !strings.Contains(err.Error(), "00") {
		t.Fatalf("a folder without its first part: %v", err)
	}
}
