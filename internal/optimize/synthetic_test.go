//go:build cgo

package optimize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/optimize/native"
	"github.com/DeLFuS77/NXPress/internal/testkit"
)

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
	if _, err := Run(context.Background(), []string{base}, target, Options{}); !errors.As(err, &notPair) {
		t.Fatalf("a game without its update: %v", err)
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
