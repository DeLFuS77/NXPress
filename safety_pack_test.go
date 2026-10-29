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

// safetyWatcher looks at the folders when a pack starts to be written, for its hidden file.
type safetyWatcher struct {
	quiet
	folders []string
	seen    map[string][]string
}

func (w *safetyWatcher) progress(path string, step string) func(int64, int64) {
	return func(int64, int64) {
		if w.seen != nil {
			return
		}
		w.seen = map[string][]string{}
		for _, folder := range w.folders {
			w.seen[folder] = safetyList(folder)
		}
	}
}

// The file chosen for a pack is where it goes, whatever the output folder: written there under
// a hidden name, never among its files, and the name is looked at before anything is written.
func TestPackGoesWhereItIsChosen(t *testing.T) {
	safetyIsolate(t)
	root := t.TempDir()
	games := filepath.Join(root, "games")
	base, update, dlc := safetyGame(t, games)
	chosen := filepath.Join(root, "sd", "Game.nsp")
	out := &safetyWatcher{folders: []string{games, filepath.Dir(chosen)}}
	if err := pack(context.Background(), options{pack: chosen, output: filepath.Join(root, "other"), files: []string{base, update, dlc}}, out); err != nil {
		t.Fatal(err)
	}
	if len(out.seen[games]) != 3 || len(out.seen[filepath.Dir(chosen)]) != 1 || !strings.HasPrefix(out.seen[filepath.Dir(chosen)][0], ".nxpress-pack-") {
		t.Fatalf("while written: %v", out.seen)
	}
	if got := safetyTitles(t, chosen); !reflect.DeepEqual(got, safetyWholeGame) || safetyExists(filepath.Join(root, "other")) {
		t.Fatalf("the pack holds %v; other folder there %v", got, safetyExists(filepath.Join(root, "other")))
	}

	// the name taken: nothing is written
	if err := pack(context.Background(), options{pack: chosen, files: []string{base, update, dlc}}, &safetyWatcher{}); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Fatalf("onto a pack there: %v", err)
	}
	// the file chosen given among the files is not one of them
	err := pack(context.Background(), options{pack: base, overwrite: true, rmSource: true, files: []string{base, dlc}}, quiet{})
	if err == nil || !safetyExists(base) || !safetyExists(dlc) {
		t.Fatalf("a pack of itself: %v; left: %v", err, safetyList(games))
	}
	if got := safetyTitles(t, base); len(got) != 1 {
		t.Fatalf("the game holds %v", got)
	}
}

// Packing a folder by game again, with the pack made before among its files: without -w the
// game is told combined already, and nothing is written.
func TestPackGamesAgainIsCombinedAlready(t *testing.T) {
	safetyIsolate(t)
	folder := t.TempDir()
	safetyGame(t, folder)
	if failed := packGames(context.Background(), options{packGames: true, files: []string{folder}}, quiet{}); failed != 0 {
		t.Fatalf("first run: %d failed", failed)
	}
	before := safetyList(folder)
	out := &safetyResults{}
	if failed := packGames(context.Background(), options{packGames: true, rmSource: true, files: []string{folder}}, out); failed != 0 {
		t.Fatalf("second run: %d failed: %v", failed, out.results)
	}
	for path, result := range out.results {
		if !isSkipped(result.err) || !strings.HasPrefix(result.err.Error(), "already combined in ") {
			t.Errorf("%s: %+v", filepath.Base(path), result)
		}
	}
	if after := safetyList(folder); !reflect.DeepEqual(after, before) {
		t.Fatalf("before %v, after %v", before, after)
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
