package main

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// safetyTicket is a ticket of a test, filled with one byte.
func safetyTicket(fill byte) []byte {
	data := make([]byte, 0x2C0)
	for i := range data {
		data[i] = fill
	}
	return data
}

// A SuperNSP with two versions of an update, packed with a DLC and --rm-source: the pack
// loses the ticket of the update it keeps, so the originals stay, and the note says why.
func TestPackKeepsTheOriginalsWhenItLacksATicket(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	ticket := "0100000000010800000000000000000a.tik"
	base := safetyTitleNsp(t, filepath.Join(folder, "b.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	v1 := safetyTitleNsp(t, filepath.Join(folder, "u1.nsp"), safetyUpdateID, 65536, switchfs.ContentMetaType_Patch, 2, map[string][]byte{ticket: safetyTicket(1)})
	v2 := safetyTitleNsp(t, filepath.Join(folder, "u2.nsp"), safetyUpdateID, 131072, switchfs.ContentMetaType_Patch, 4, map[string][]byte{ticket: safetyTicket(2)})
	super := filepath.Join(folder, "super.nsp")
	if _, _, err := switchfs.MergeAndCheck(context.Background(), []string{base, v2, v1}, super, true, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(base)
	os.Remove(v1)
	os.Remove(v2)
	dlc := safetyTitleNsp(t, filepath.Join(folder, "d.nsp"), safetyDlcID, 0, switchfs.ContentMetaType_AddOnContent, 3, nil)
	packed := filepath.Join(folder, "pack.nsp")
	out := &safetyResults{}
	if err := pack(context.Background(), options{pack: packed, rmSource: true, files: []string{super, dlc}}, out); err != nil {
		t.Fatal(err)
	}
	lacks, err := packLacks(packed, []string{super, dlc})
	if err != nil || !reflect.DeepEqual(lacks, []string{"the ticket of 0100000000010800"}) {
		t.Fatalf("the pack lacks %v, %v", lacks, err)
	}
	if !safetyExists(super) || !safetyExists(dlc) {
		t.Fatalf("an original was deleted: %v", safetyList(folder))
	}
	if note := out.results[packed].note; !strings.Contains(note, "the originals are kept: the pack lacks the ticket of 0100000000010800") {
		t.Fatalf("note %q", note)
	}
}

// A pack that holds everything of its titles deletes its originals with --rm-source.
func TestPackWithEverythingDeletesItsOriginals(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	base := safetyTitleNsp(t, filepath.Join(folder, "b.nsp"), safetyBaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	update := safetyTitleNsp(t, filepath.Join(folder, "u.nsp"), safetyUpdateID, 65536, switchfs.ContentMetaType_Patch, 2, map[string][]byte{"0100000000010800000000000000000a.tik": safetyTicket(1)})
	packed := filepath.Join(folder, "pack.nsp")
	if err := pack(context.Background(), options{pack: packed, rmSource: true, files: []string{base, update}}, quiet{}); err != nil {
		t.Fatal(err)
	}
	if left := safetyList(folder); !reflect.DeepEqual(left, []string{"pack.nsp"}) {
		t.Fatalf("left: %v", left)
	}
	if lacks, err := packLacks(packed, nil); err != nil || len(lacks) != 0 {
		t.Fatalf("the pack lacks %v, %v", lacks, err)
	}
}

// The files of a game card are the ones of its secure partition.
func TestPackageFilesOfAGameCard(t *testing.T) {
	hfs0 := func(names []string, sizes []int) []byte {
		table := []byte{}
		at := []int{}
		for _, name := range names {
			at = append(at, len(table))
			table = append(table, append([]byte(name), 0)...)
		}
		head := make([]byte, 0x10+0x40*len(names))
		copy(head, "HFS0")
		binary.LittleEndian.PutUint32(head[4:], uint32(len(names)))
		binary.LittleEndian.PutUint32(head[8:], uint32(len(table)))
		offset := 0
		for i := range names {
			entry := head[0x10+0x40*i:]
			binary.LittleEndian.PutUint64(entry[0:], uint64(offset))
			binary.LittleEndian.PutUint64(entry[8:], uint64(sizes[i]))
			binary.LittleEndian.PutUint32(entry[16:], uint32(at[i]))
			offset += sizes[i]
		}
		return append(head, table...)
	}
	secure := hfs0([]string{"0123456789abcdef0123456789abcdef.nca", "0100000000010800000000000000000a.tik"}, []int{0x10, 0x10})
	secure = append(secure, make([]byte, 0x20)...)
	root := hfs0([]string{"update", "secure"}, []int{0, len(secure)})
	card := make([]byte, 0x200)
	copy(card[0x100:], "HEAD")
	binary.LittleEndian.PutUint64(card[0x130:], 0x200)
	card = append(append(card, root...), secure...)
	path := filepath.Join(t.TempDir(), "card.xci")
	os.WriteFile(path, card, 0o644)
	names, err := packageFiles(path)
	if err != nil || !reflect.DeepEqual(names, []string{"0123456789abcdef0123456789abcdef.nca", "0100000000010800000000000000000a.tik"}) {
		t.Fatalf("%v %v", names, err)
	}
}
