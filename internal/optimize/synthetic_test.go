//go:build cgo

package optimize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/optimize/native"
	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The DLC of the game among the files given are found with every NCA they list, the newest of
// each: they go into the optimized NSP as they are. A DLC of another game is not one of them, and a
// DLC whose NCA is not given is told.
func TestFindsTheDlcOfTheGame(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, dlc := testkit.Game(t, folder)
	older := testkit.TitleNsp(t, filepath.Join(folder, "older.nsp"), testkit.DlcID, 0, switchfs.ContentMetaType_AddOnContent, 5, nil)
	newer := testkit.TitleNsp(t, filepath.Join(folder, "newer.nsp"), testkit.DlcID, 65536, switchfs.ContentMetaType_AddOnContent, 6, nil)
	other := testkit.TitleNsp(t, filepath.Join(folder, "other.nsp"), testkit.BaseID+0x11001, 0, switchfs.ContentMetaType_AddOnContent, 4, nil)
	k, err := loadKeys()
	if err != nil {
		t.Fatal(err)
	}
	read := func(paths ...string) *game {
		pkgs := []*pkgFile{}
		for _, path := range paths {
			p, err := openNsp(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Close() })
			pkgs = append(pkgs, p)
		}
		g, err := readGame(k, pkgs, readCnmt)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	g := read(base, update, dlc, other)
	b, _, err := g.pair()
	if err != nil {
		t.Fatal(err)
	}
	dlcs, err := g.dlcs(b)
	if err != nil || len(dlcs) != 1 || dlcs[0].titleID != testkit.DlcID {
		t.Fatalf("DLC of the game: %v, %d", err, len(dlcs))
	}
	if files := dlcContents(g, dlcs[0]); len(files) != len(dlcs[0].contents) || len(files) == 0 {
		t.Fatalf("its NCAs: %d", len(files))
	}
	// two versions of it: the newest
	g = read(base, older, newer)
	b, _, _ = g.pair()
	if dlcs, err := g.dlcs(b); err != nil || len(dlcs) != 1 || dlcs[0].version != 65536 {
		t.Fatalf("the newest DLC: %v, %v", err, dlcs)
	}
	// an NCA it lists, not given
	g = read(base, dlc)
	b, _, _ = g.pair()
	dlcs, _ = g.dlcs(b)
	delete(g.ncas, dlcs[0].contents[0].id)
	if _, err := g.dlcs(b); err == nil || !strings.Contains(err.Error(), "is not among the files given") {
		t.Fatalf("a DLC without its NCA: %v", err)
	}
}

// The synthetic game of the tests read as a merge reads it: the headers decrypted with the
// header key, the meta NCAs by their key area, their CNMT read by the code of NXDock, and the game
// and its update found with their programs.
func TestReadsTheGameAndItsUpdate(t *testing.T) {
	testkit.Isolate(t)
	base, update, dlc := testkit.Game(t, t.TempDir())
	k, err := loadKeys()
	if err != nil {
		t.Fatal(err)
	}
	pkgs := []*pkgFile{}
	for _, path := range []string{dlc, update, base} {
		p, err := openNsp(path)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		pkgs = append(pkgs, p)
	}
	g, err := readGame(k, pkgs, readCnmt)
	if err != nil {
		t.Fatal(err)
	}
	b, u, err := g.pair()
	if err != nil {
		t.Fatal(err)
	}
	if b.titleID != testkit.BaseID || b.typ != metaApplication || u.titleID != testkit.UpdateID || u.typ != metaPatch || u.version != 65536 {
		t.Fatalf("base %016X %x, update %016X %x v%d", b.titleID, b.typ, u.titleID, u.typ, u.version)
	}
	for _, m := range []*meta{b, u} {
		if prog, programs, err := g.program(m, 0); err != nil || programs != 1 || prog.contentType() != 0 {
			t.Fatalf("program of %016X: %v, %d programs", m.titleID, err, programs)
		}
	}
}

// A game and an "update" that is no patch of it (the synthetic one is a whole program) are not
// a pair: told so, and nothing is left behind.
func TestRunTellsWhatIsNoPair(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	base, update, _ := testkit.Game(t, folder)
	target := filepath.Join(folder, "out.nsp")
	_, err := Run(context.Background(), []string{base, update}, target, Options{})
	var notPair ErrNotPair
	if !errors.As(err, &notPair) {
		t.Fatalf("told %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the NSP was left")
	}
	if left := testkit.List(folder); len(left) != 3 {
		t.Fatalf("left: %v", left)
	}
	// a game alone is made again from its own program: the synthetic one has no RomFS to make
	if _, err := Run(context.Background(), []string{base}, target, Options{}); !errors.As(err, &notPair) || !strings.Contains(err.Error(), "no RomFS") {
		t.Fatalf("a game alone with no RomFS: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the NSP was left")
	}
}

// What is not a CNMT, and NCAs that are no RomFS, are told, not read past their end.
func TestNativeTellsDamage(t *testing.T) {
	if _, err := native.ReadCnmt(make([]byte, 0xC00), make([]byte, 0x200)); err == nil {
		t.Fatal("a CNMT of zeros read")
	}
	job := &native.Job{}
	if _, err := native.OpenRomfs(job, native.Section{End: 0x4000}, nil); err == nil {
		t.Fatal("a RomFS opened without an NCA")
	}
	if _, err := native.Open(job, native.Section{End: 0x4000}, native.Section{End: 0x4000}, 1, make([]byte, 0xC00), 9, 1); err == nil {
		t.Fatal("a merge opened of nothing")
	}
}
