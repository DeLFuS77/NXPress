package nup

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testnup"
)

// zeroBased is a game and its update whose metadata a build of the packer of the format wrote
// with its types counted from 0, and a DLC with names by language.
func zeroBased() []testnup.Package {
	packages := playable()
	packages[0].Meta = []testnup.Record{{Type: 0, Payload: npdm(0x0400486024DEC000)}, {Type: 2, Payload: testnup.Jpeg},
		{Type: 3, Payload: []byte("1.0.0")}, {Type: 4, Payload: []byte("Made-up Quest")}, {Type: 5, Payload: []byte("Nobody Games")},
		{Type: 7, Payload: []byte("Made-up Dump")}}
	packages[1].Meta = []testnup.Record{{Type: 0, Payload: npdm(0x0400486024DEC000)}, {Type: 3, Payload: []byte("1.0.2")}}
	return packages
}

func TestFixZeroBased(t *testing.T) {
	path := write(t, testnup.Build(zeroBased()))
	before, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Emulators().ZeroBased || before.Packages[0].Name() != "Made-up Quest" {
		t.Fatalf("%+v", before.Packages[0])
	}
	fixed, err := FixZeroBased(path)
	if err != nil || fixed != 2 {
		t.Fatalf("%d %v", fixed, err)
	}
	after, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	game := after.Packages[0]
	if after.Emulators().ZeroBased || game.Name() != "Made-up Quest" || game.Publisher() != "Nobody Games" ||
		game.DisplayVersion() != "1.0.0" || !bytes.Equal(game.Icon(), testnup.Jpeg) || after.Packages[1].DisplayVersion() != "1.0.2" {
		t.Errorf("after: %+v", after.Packages)
	}
	// as the emulators read it: the name where the format puts it
	if string(game.Meta[tlvTitleName]) != "Made-up Quest" || string(game.Meta[tlvPublisher]) != "Nobody Games" {
		t.Errorf("records %v", game.Meta)
	}
	// the files are as they were, and the copy beside it is gone
	if _, check, err := Verify(context.Background(), path, 2, 2, nil); err != nil || len(check.Damaged) != 0 {
		t.Errorf("%v %+v", err, check)
	}
	if _, err := os.Stat(MetaBackup(path)); !os.IsNotExist(err) {
		t.Errorf("backup left: %v", err)
	}
	// again: nothing to do
	if fixed, err := FixZeroBased(path); err != nil || fixed != 0 {
		t.Errorf("again: %d %v", fixed, err)
	}
	// a NUP counted from 1 is left as it is
	good := write(t, testnup.Build(playable()))
	original, _ := os.ReadFile(good)
	if fixed, err := FixZeroBased(good); err != nil || fixed != 0 {
		t.Errorf("counted from 1: %d %v", fixed, err)
	}
	if now, _ := os.ReadFile(good); !bytes.Equal(now, original) {
		t.Error("a NUP counted from 1 was written")
	}
}

// A copy left beside a NUP by a fix that stopped is put back first; the copy of another NUP stops
// the work and is left where it is.
func TestFixZeroBasedAfterAStop(t *testing.T) {
	original := testnup.Build(zeroBased())
	path := write(t, original)
	if _, err := FixZeroBased(path); err != nil {
		t.Fatal(err)
	}
	fixed, _ := os.ReadFile(path)
	// the copy a fix that stopped after writing the first block leaves: the blocks as they were
	container, err := Read(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	backup := []byte(backupMagic)
	backup = appendUint64(backup, uint64(len(original)))
	backup = append(backup, original[0x18:0x38]...)
	backup = appendUint32(backup, 2)
	for _, pkg := range container.Packages[:2] {
		backup = appendUint64(backup, pkg.metaOffset)
		backup = appendUint32(backup, uint32(pkg.metaSize))
		backup = append(backup, original[pkg.metaOffset:pkg.metaOffset+pkg.metaSize]...)
	}
	half := bytes.Clone(fixed)
	second := container.Packages[1]
	copy(half[second.metaOffset:], original[second.metaOffset:second.metaOffset+second.metaSize])
	os.WriteFile(path, half, 0o644)
	os.WriteFile(MetaBackup(path), backup, 0o644)
	if n, err := FixZeroBased(path); err != nil || n != 2 {
		t.Fatalf("after a stop: %d %v", n, err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, fixed) {
		t.Error("not the same as fixed at once")
	}

	// the copy of another NUP
	os.WriteFile(path, original, 0o644)
	os.WriteFile(MetaBackup(path), append([]byte(backupMagic), make([]byte, 64)...), 0o644)
	if _, err := FixZeroBased(path); err == nil || !strings.Contains(err.Error(), "another NUP") {
		t.Errorf("another NUP: %v", err)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, original) {
		t.Error("written with the copy of another NUP")
	}
	if _, err := os.Stat(MetaBackup(path)); err != nil {
		t.Errorf("the copy of another NUP removed: %v", err)
	}
}

func appendUint64(b []byte, v uint64) []byte {
	for i := 0; i < 8; i++ {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}

func appendUint32(b []byte, v uint32) []byte {
	for i := 0; i < 4; i++ {
		b = append(b, byte(v>>(8*i)))
	}
	return b
}
